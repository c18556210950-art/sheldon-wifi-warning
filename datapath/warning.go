package datapath

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"
	"gvisor.dev/gvisor/pkg/waiter"
)

//go:embed warning_assets/*
var warningAssets embed.FS

const warningIP = "198.18.0.1"
const warningURL = "http://198.18.0.1/"

// warningGate deliberately has shared state: Android NAT merges visitor IPv4
// identities. One visitor releases this batch, and a new association rearms it.
type warningGate struct {
	mu               sync.Mutex
	enabled, pending bool
	token            string
	generation       uint64
}

func newWarningGate() *warningGate {
	g := &warningGate{enabled: true}
	g.rearm()
	return g
}
func (g *warningGate) rearm() {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("warning token entropy unavailable")
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.token = hex.EncodeToString(b[:])
	g.generation++
	g.pending = g.enabled
}
func (g *warningGate) blocked() bool {
	if g == nil {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.enabled && g.pending
}
func (g *warningGate) setEnabled(enabled bool) {
	g.mu.Lock()
	g.enabled = enabled
	if !enabled {
		g.pending = false
	}
	g.mu.Unlock()
	if enabled {
		g.rearm()
	}
}
func (g *warningGate) status() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	b, _ := json.Marshal(map[string]interface{}{"enabled": g.enabled, "pending": g.pending, "generation": g.generation, "mode": "shared", "url": warningURL})
	return string(b)
}
func (g *warningGate) release(token string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if token == "" || token != g.token {
		return false
	}
	g.pending = false
	return true
}
func (g *warningGate) currentToken() string { g.mu.Lock(); defer g.mu.Unlock(); return g.token }

// handleTCP consumes only portal traffic and connections pending a warning.
// TLS is never redirected or impersonated: pending HTTPS is refused, allowing
// the device's HTTP captive-network probe to discover the warning page.
func (g *warningGate) handleTCP(request *tcp.ForwarderRequest) bool {
	if g == nil {
		return false
	}
	id := request.ID()
	isPortal := addressString(id.LocalAddress) == warningIP
	if !isPortal && !g.blocked() {
		return false
	}
	if id.LocalPort != 80 {
		request.Complete(true)
		return true
	}
	var queue waiter.Queue
	endpoint, err := request.CreateEndpoint(&queue)
	if err != nil {
		request.Complete(true)
		return true
	}
	request.Complete(false)
	client := gonet.NewTCPConn(&queue, endpoint)
	go g.serveConnection(client, isPortal)
	return true
}
func (g *warningGate) serveConnection(conn net.Conn, isPortal bool) {
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(10 * time.Second))
	req, err := http.ReadRequest(bufio.NewReader(io.LimitReader(conn, 20*1024)))
	if err != nil {
		return
	}
	defer req.Body.Close()
	rec := httptest.NewRecorder()
	if isPortal {
		g.ServeHTTP(rec, req)
	} else if g.blocked() {
		rec.Header().Set("Cache-Control", "no-store")
		http.Redirect(rec, req, warningURL, http.StatusFound)
	} else {
		// A batch can be released while a canary request is being parsed.
		rec.Header().Set("Content-Type", "text/html")
		rec.Write([]byte("<HTML><HEAD><TITLE>Success</TITLE></HEAD><BODY>Success</BODY></HTML>"))
	}
	response := rec.Result()
	response.Close = true
	defer response.Body.Close()
	response.Write(conn)
}
func (g *warningGate) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; frame-ancestors 'none'")
	switch r.URL.Path {
	case "/continue":
		if r.Method != "POST" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1024)
		if err := r.ParseForm(); err != nil || !g.release(r.PostForm.Get("token")) {
			http.Error(w, "警告已更新，请刷新页面后重试。", http.StatusConflict)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(w, "<!doctype html><meta charset=utf-8><meta name=viewport content='width=device-width,initial-scale=1'><title>已放行</title><p>警告结束，已恢复上网，可以关闭本页。</p>")
	case "/", "/index.html":
		if r.Method != "GET" && r.Method != "HEAD" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		body, _ := warningAssets.ReadFile("warning_assets/index.html")
		content := strings.ReplaceAll(string(body), "{{TOKEN}}", g.currentToken())
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		http.ServeContent(w, r, "index.html", time.Time{}, strings.NewReader(content))
	case "/warning.m4a", "/config.json":
		if r.Method != "GET" && r.Method != "HEAD" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/")
		data, err := warningAssets.ReadFile("warning_assets/" + name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		mime := "application/json; charset=utf-8"
		if name == "warning.m4a" {
			mime = "audio/mp4"
		}
		w.Header().Set("Content-Type", mime)
		http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
	default:
		http.NotFound(w, r)
	}
}
func (s *Session) RearmWarning() {
	if s.warning != nil {
		s.warning.rearm()
	}
}
func (s *Session) SetWarningEnabled(enabled bool) {
	if s.warning != nil {
		s.warning.setEnabled(enabled)
	}
}
func (s *Session) WarningStatus() string {
	if s.warning == nil {
		return "{}"
	}
	return s.warning.status()
}
