package openssh

import (
	"os"
	"reflect"
	"testing"
)

func TestControlArgsDoNotLoadUserConfig(t *testing.T) {
	master := Master{Host: "prod", ControlPath: "/tmp/cbssh.sock"}
	got := controlArgs(master, "forward", "-L", "127.0.0.1:8080:db:80")
	want := []string{"-F", os.DevNull, "-S", "/tmp/cbssh.sock", "-O", "forward", "-o", "ExitOnForwardFailure=yes", "-L", "127.0.0.1:8080:db:80", "prod"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("controlArgs() = %#v, want %#v", got, want)
	}
}
