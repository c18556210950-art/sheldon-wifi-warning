// Package datapath terminates tethered client traffic in userspace.
//
// The tethering stack routes hotspot clients into a TUN owned by the shell
// process. Nothing in the kernel forwards those packets onward, so this package
// reads raw IP frames off the TUN fd, terminates TCP and UDP in a gVisor
// netstack instance, and proxies each flow onto an ordinary socket.
package datapath

import (
	"encoding/json"
	"net"
	"sync"

	"golang.org/x/sys/unix"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
)

// nicID identifies the single TUN-backed interface in the stack.
const nicID tcpip.NICID = 1

// Session owns a running netstack bound to one TUN file descriptor. Exposes no
// gVisor types: gomobile carries only a narrow set across JNI, so the Kotlin
// side sees a handle it can start and stop.
type Session struct {
	stack    *stack.Stack
	binding  *networkBinding
	ping     *pingForwarder
	udp      *udpMapper
	identity *identityObservation
	warning  *warningGate
}

// Start builds a netstack over tunFD, already open, and attaches it to the TUN.
// Ownership does not transfer -- SessionResources holds the TUN, the framework
// interface, and the network handle as one atomic group. mtu is the link MTU.
func Start(tunFD int, mtu int) (*Session, error) {
	netStack, err := newNetStack(tunFD, mtu)
	if err != nil {
		return nil, err
	}

	binding := &networkBinding{}
	identity := &identityObservation{sources: make(map[string]bool)}
	warning := newWarningGate()
	udp := installForwardersWithGate(netStack, &net.Dialer{
		Timeout: dialTimeout,
		Control: binding.control,
	}, binding.handle.Load, warning, identity.observe)

	// Echo replies go straight onto the TUN: they are whole IPv4 packets, and
	// the kernel side routes them back through tethering's NAT like any reply.
	ping := newPingForwarder(binding.echo, func(packet []byte) error {
		_, err := unix.Write(tunFD, packet)
		return err
	})
	ping.install(netStack)

	return &Session{stack: netStack, binding: binding, ping: ping, udp: udp, identity: identity, warning: warning}, nil
}

// SetNetwork pins every subsequent dial to a handle from
// Network.getNetworkHandle; 0 unbinds, which is how a session with no VPN runs.
//
// Binding is what makes VPN loss fail rather than fall back: an unbound socket
// follows the default network, so a dropped VPN leaves over the physical one
// and tethered clients keep browsing untunneled with nothing noticing.
//
// int64 because gomobile has no unsigned type. A handle packs a netid above a
// magic word, so it is routinely negative signed; the conversion reinterprets
// the bits without changing them.
func (s *Session) SetNetwork(handle int64) {
	if s.binding == nil {
		return
	}
	s.binding.set(uint64(handle))

	// UDP mappings stay pinned to the network they opened on.
	if s.udp != nil {
		s.udp.retireAll()
	}
}

// Stop tears the netstack down. It does not close the TUN fd.
func (s *Session) Stop() {
	if s.stack == nil {
		return
	}
	// Fence ping replies first: they write to the raw TUN fd, which the caller
	// closes as soon as Stop returns.
	if s.ping != nil {
		s.ping.close()
	}
	if s.udp != nil {
		s.udp.close()
	}
	s.stack.Close()
	s.stack = nil
}

// identityObservation measures what survives Android tethering NAT. Never use
// the shared TUN address as a device identity or a per-device whitelist key.
type identityObservation struct {
	mu            sync.Mutex
	sources       map[string]bool
	sawSharedIPv4 bool
}

func (o *identityObservation) observe(id stack.TransportEndpointID) {
	source := addressString(id.RemoteAddress)
	o.mu.Lock()
	defer o.mu.Unlock()
	if len(o.sources) < 128 {
		o.sources[source] = true
	}
	if source == "192.0.2.2" {
		o.sawSharedIPv4 = true
	}
}

// IdentityStatus reports only counts and the NAT limitation, not browsing data.
func (s *Session) IdentityStatus() string {
	o := s.identity
	if o == nil {
		return `{}`
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	data, _ := json.Marshal(map[string]interface{}{
		"observedSourceCount":           len(o.sources),
		"sharedIPv4SourceObserved":      o.sawSharedIPv4,
		"perDeviceIPv4PoliciesVerified": false,
	})
	return string(data)
}
