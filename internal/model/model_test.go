package model

import "testing"

func TestTunnelTargetAddressEmptyForDynamicTunnel(t *testing.T) {
	tun := Tunnel{
		Name:       "socks",
		Type:       TunnelTypeDynamic,
		ListenHost: "127.0.0.1",
		ListenPort: 1080,
	}
	if got := tun.TargetAddress(); got != "" {
		t.Fatalf("TargetAddress() = %q, want empty", got)
	}
}
