package filetransfer

import (
	"os"
	"time"
)

// Options describes user-facing transfer behavior shared by uploads and downloads.
type Options struct {
	Recursive bool
	Force     bool
}

// Result summarizes one completed upload or download operation.
type Result struct {
	HostName    string
	LocalPath   string
	RemotePath  string
	Files       int
	Directories int
	Bytes       int64
}

// Entry is one remote directory entry returned by ListDir.
type Entry struct {
	Name    string
	Path    string
	IsDir   bool
	Size    int64
	Mode    os.FileMode
	ModTime time.Time
}
