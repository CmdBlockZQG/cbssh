package tunnel

import "testing"

func TestSupportsSOCKSNoAuth(t *testing.T) {
	if !supportsSOCKSNoAuth([]byte{0x02, 0x00}) {
		t.Fatal("supportsSOCKSNoAuth() = false, want true")
	}
	if supportsSOCKSNoAuth([]byte{0x02}) {
		t.Fatal("supportsSOCKSNoAuth() = true, want false")
	}
}
