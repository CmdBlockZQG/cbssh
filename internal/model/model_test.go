package model

import "testing"

func TestForwardSpec(t *testing.T) {
	tests := []struct {
		name string
		tun  Tunnel
		flag string
		spec string
	}{
		{"local", Tunnel{Type: TunnelTypeLocal, BindHost: "127.0.0.1", BindPort: 15432, TargetHost: "db", TargetPort: 5432}, "-L", "127.0.0.1:15432:db:5432"},
		{"remote", Tunnel{Type: TunnelTypeRemote, BindHost: "::1", BindPort: 8080, TargetHost: "127.0.0.1", TargetPort: 80}, "-R", "[::1]:8080:127.0.0.1:80"},
		{"dynamic", Tunnel{Type: TunnelTypeDynamic, BindHost: "127.0.0.1", BindPort: 1080}, "-D", "127.0.0.1:1080"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.tun.ForwardSpec()
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.spec || tt.tun.ForwardFlag() != tt.flag {
				t.Fatalf("forward = %s %s, want %s %s", tt.tun.ForwardFlag(), got, tt.flag, tt.spec)
			}
		})
	}
}
