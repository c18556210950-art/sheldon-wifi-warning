package datapath

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"gvisor.dev/gvisor/pkg/buffer"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/link/channel"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
)

// Exercise actual TCP packets through the same netstack forwarder used by the
// phone, not only the HTTP handler: captive probe -> page -> release -> internet.
func TestWarningTCPFlow(t *testing.T) {
	server, sl := newPingTestStack(t)
	client, cl := newPingTestStack(t)
	source := tcpip.AddrFrom4([4]byte{10, 105, 0, 100})
	if err := client.AddProtocolAddress(nicID, tcpip.ProtocolAddress{Protocol: header.IPv4ProtocolNumber, AddressWithPrefix: source.WithPrefix()}, stack.AddressProperties{}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	bridge := func(from, to *channel.Endpoint) {
		for {
			p := from.ReadContext(ctx)
			if p == nil {
				return
			}
			v := p.ToView()
			copy := stack.NewPacketBuffer(stack.PacketBufferOptions{Payload: buffer.MakeWithData(append([]byte(nil), v.AsSlice()...))})
			to.InjectInbound(p.NetworkProtocolNumber, copy)
			copy.DecRef()
			v.Release()
			p.DecRef()
		}
	}
	go bridge(sl, cl)
	go bridge(cl, sl)
	gate := newWarningGate()
	mapper := installForwardersWithGate(server, &net.Dialer{Timeout: time.Second}, func() uint64 { return 0 }, gate)
	t.Cleanup(mapper.close)
	request := func(ip [4]byte, port uint16, path string, method, body string) *http.Response {
		t.Helper()
		timeout, done := context.WithTimeout(ctx, 3*time.Second)
		defer done()
		conn, err := gonet.DialTCPWithBind(timeout, client, tcpip.FullAddress{NIC: nicID, Addr: source}, tcpip.FullAddress{NIC: nicID, Addr: tcpip.AddrFrom4(ip), Port: port}, header.IPv4ProtocolNumber)
		if err != nil {
			t.Fatal(err)
		}
		conn.SetDeadline(time.Now().Add(3 * time.Second))
		req, _ := http.NewRequest(method, "http://portal"+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if err := req.Write(conn); err != nil {
			conn.Close()
			t.Fatal(err)
		}
		response, err := http.ReadResponse(bufio.NewReader(conn), req)
		if err != nil {
			conn.Close()
			t.Fatal(err)
		}
		t.Cleanup(func() { response.Body.Close(); conn.Close() })
		return response
	}
	probe := request([4]byte{1, 1, 1, 1}, 80, "/generate_204", "GET", "")
	io.Copy(io.Discard, probe.Body)
	if probe.StatusCode != 302 || probe.Header.Get("Location") != warningURL {
		t.Fatal(probe.Status)
	}
	page := request([4]byte{198, 18, 0, 1}, 80, "/", "GET", "")
	html, _ := io.ReadAll(page.Body)
	if !strings.Contains(string(html), "播放警告录音") {
		t.Fatalf("missing warning page: %s", html)
	}
	release := request([4]byte{198, 18, 0, 1}, 80, "/continue", "POST", "token="+gate.currentToken())
	io.Copy(io.Discard, release.Body)
	if release.StatusCode != 200 || gate.blocked() {
		t.Fatal("release failed")
	}
	ip := localIPv4(t)
	listener, err := net.Listen("tcp4", net.JoinHostPort(ip.String(), "0"))
	if err != nil {
		t.Fatal(err)
	}
	upstream := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "internet restored") })}
	go upstream.Serve(listener)
	t.Cleanup(func() { upstream.Close() })
	port := uint16(listener.Addr().(*net.TCPAddr).Port)
	online := request(ip.As4(), port, "/", "GET", "")
	content, _ := io.ReadAll(online.Body)
	if string(content) != "internet restored" {
		t.Fatalf("upstream not restored: %s", content)
	}
	gate.rearm()
	again := request([4]byte{1, 1, 1, 1}, 80, "/generate_204", "GET", "")
	if again.StatusCode != 302 {
		t.Fatal("re-arm did not intercept next probe")
	}
	io.Copy(io.Discard, again.Body)
}
