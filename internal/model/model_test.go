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

func TestTunnelTypeCode(t *testing.T) {
	tests := []struct {
		value string
		want  string
	}{
		{TunnelTypeDynamic, "D"},
		{TunnelTypeLocal, "L"},
		{TunnelTypeRemote, "R"},
		{"custom", "custom"},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			if got := TunnelTypeCode(tt.value); got != tt.want {
				t.Fatalf("TunnelTypeCode(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}
