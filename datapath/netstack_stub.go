//go:build !linux && !android

package datapath

import (
	"fmt"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
)

// Host unit tests use a channel endpoint; a real TUN requires Android/Linux.
func newNetStack(tunFD int, mtu int) (*stack.Stack, error) {
	return nil, fmt.Errorf("TUN datapath requires Android or Linux")
}
