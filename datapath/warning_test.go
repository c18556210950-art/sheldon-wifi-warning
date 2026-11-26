package datapath

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestWarningReleaseRearmAndAudioRange(t *testing.T) {
	g := newWarningGate()
	if !g.blocked() {
		t.Fatal("initial warning inactive")
	}
	page := httptest.NewRecorder()
	g.ServeHTTP(page, httptest.NewRequest("GET", warningURL, nil))
	if page.Code != 200 || !strings.Contains(page.Body.String(), g.currentToken()) {
		t.Fatal("missing release token")
	}
	audio := httptest.NewRecorder()
	r := httptest.NewRequest("GET", warningURL+"warning.m4a", nil)
	r.Header.Set("Range", "bytes=0-99")
	g.ServeHTTP(audio, r)
	if audio.Code != 206 || audio.Body.Len() != 100 || audio.Header().Get("Content-Type") != "audio/mp4" {
		t.Fatalf("range broken: %d %v", audio.Code, audio.Header())
	}
	old := g.currentToken()
	g.rearm()
	if g.release(old) {
		t.Fatal("stale token released new warning")
	}
	post := httptest.NewRequest("POST", warningURL+"continue", strings.NewReader(url.Values{"token": {g.currentToken()}}.Encode()))
	post.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	g.ServeHTTP(response, post)
	if response.Code != 200 || g.blocked() {
		t.Fatal("continue did not restore access")
	}
	g.rearm()
	if !g.blocked() {
		t.Fatal("reconnection did not rearm")
	}
	g.setEnabled(false)
	g.rearm()
	if g.blocked() {
		t.Fatal("rearm ignored disabled setting")
	}
}
func TestCanaryRequestRedirectedOverConnection(t *testing.T) {
	g := newWarningGate()
	server, client := net.Pipe()
	go g.serveConnection(server, false)
	req, _ := http.NewRequest("GET", "http://connectivitycheck.gstatic.com/generate_204", nil)
	go req.Write(client)
	response, err := http.ReadResponse(bufio.NewReader(client), req)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	defer response.Body.Close()
	io.Copy(io.Discard, response.Body)
	if response.StatusCode != 302 || response.Header.Get("Location") != warningURL {
		t.Fatal(response.Status)
	}
}
