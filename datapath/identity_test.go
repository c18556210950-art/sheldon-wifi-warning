package datapath

import (
	"encoding/json"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"testing"
)

func TestNATSourceCannotAuthorizeDevices(t *testing.T) {
	o := &identityObservation{sources: make(map[string]bool)}
	s := &Session{identity: o}
	for _, port := range []uint16{12345, 12346} {
		o.observe(stack.TransportEndpointID{RemoteAddress: tcpip.AddrFrom4([4]byte{192, 0, 2, 2}), RemotePort: port})
	}
	var report map[string]interface{}
	if err := json.Unmarshal([]byte(s.IdentityStatus()), &report); err != nil {
		t.Fatal(err)
	}
	if report["observedSourceCount"] != float64(1) || report["sharedIPv4SourceObserved"] != true || report["perDeviceIPv4PoliciesVerified"] != false {
		t.Fatalf("unsafe identity report: %v", report)
	}
}
func TestNoTrafficDoesNotClaimSupport(t *testing.T) {
	s := &Session{identity: &identityObservation{sources: make(map[string]bool)}}
	var report map[string]interface{}
	json.Unmarshal([]byte(s.IdentityStatus()), &report)
	if report["perDeviceIPv4PoliciesVerified"] != false {
		t.Fatal(report)
	}
}
