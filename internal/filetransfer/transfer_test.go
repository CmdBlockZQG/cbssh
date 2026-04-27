package filetransfer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDownloadFileDestinationDefaultsToRemoteBase(t *testing.T) {
	got, err := downloadFileDestination("/var/log/app.log", "")
	if err != nil {
		t.Fatalf("downloadFileDestination error = %v", err)
	}
	if got != "app.log" {
		t.Fatalf("downloadFileDestination = %q, want %q", got, "app.log")
	}
}

func TestDownloadFileDestinationUsesExistingDirectory(t *testing.T) {
	dir := t.TempDir()
	got, err := downloadFileDestination("/var/log/app.log", dir)
	if err != nil {
		t.Fatalf("downloadFileDestination error = %v", err)
	}
	want := filepath.Join(dir, "app.log")
	if got != want {
		t.Fatalf("downloadFileDestination = %q, want %q", got, want)
	}
}

func TestDownloadFileDestinationUsesTrailingSeparatorAsDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "downloads")
	got, err := downloadFileDestination("/var/log/app.log", dir+string(os.PathSeparator))
	if err != nil {
		t.Fatalf("downloadFileDestination error = %v", err)
	}
	want := filepath.Join(dir, "app.log")
	if got != want {
		t.Fatalf("downloadFileDestination = %q, want %q", got, want)
	}
}

func TestDownloadDirDestinationDefaultsToRemoteBase(t *testing.T) {
	got := downloadDirDestination("/var/log/app", "")
	if got != "app" {
		t.Fatalf("downloadDirDestination = %q, want %q", got, "app")
	}
}

func TestDownloadDirDestinationRootNeedsExplicitLocalPath(t *testing.T) {
	got := downloadDirDestination("/", "")
	if got != "" {
		t.Fatalf("downloadDirDestination = %q, want empty", got)
	}
}

func TestRemoteJoinRelUsesPOSIXSeparators(t *testing.T) {
	rel := filepath.Join("nested", "file.txt")
	got := remoteJoinRel("/tmp/root", rel)
	want := "/tmp/root/nested/file.txt"
	if got != want {
		t.Fatalf("remoteJoinRel = %q, want %q", got, want)
	}
}

func TestRemoteBaseRootIsEmpty(t *testing.T) {
	if got := remoteBase("/"); got != "" {
		t.Fatalf("remoteBase = %q, want empty", got)
	}
}
