package openssh

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestControlArgsDoNotLoadUserConfig(t *testing.T) {
	master := Master{Host: "prod", ControlPath: "/tmp/cbssh.sock"}
	got := controlArgs(master, "forward", "-L", "127.0.0.1:8080:db:80")
	want := []string{"-F", os.DevNull, "-S", "/tmp/cbssh.sock", "-O", "forward", "-o", "ExitOnForwardFailure=yes", "-L", "127.0.0.1:8080:db:80", "prod"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("controlArgs() = %#v, want %#v", got, want)
	}
}

func TestStartMasterDoesNotWaitForProxyHelperStderr(t *testing.T) {
	// The helper simulates ProxyJump's long-lived ssh -W process. It inherits
	// stderr and keeps it open after the foreground command exits.
	tempDir := t.TempDir()
	binary := filepath.Join(tempDir, "ssh")
	release := filepath.Join(tempDir, "release.fifo")
	if err := unix.Mkfifo(release, 0o600); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\ncat '" + release + "' >&2 &\nexit 0\n"
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer devNull.Close()

	runner := &CommandRunner{Binary: binary, Stdout: devNull, Stderr: devNull}
	released := false
	releaseHelper := func() {
		if released {
			return
		}
		released = true
		file, err := os.OpenFile(release, os.O_WRONLY, 0)
		if err == nil {
			_ = file.Close()
		}
	}
	defer releaseHelper()
	done := make(chan error, 1)
	go func() {
		done <- runner.StartMaster(context.Background(), Master{Host: "target"})
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		// Release the simulated helper before failing so the test does not
		// leave a process holding the inherited descriptor behind.
		releaseHelper()
		<-done
		t.Fatal("StartMaster waited for inherited stderr")
	}
	releaseHelper()
}
