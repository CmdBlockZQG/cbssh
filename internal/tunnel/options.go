package tunnel

import (
	"errors"
	"io"
	"net"
	"time"

	"github.com/cmdblock/cbssh/internal/model"
	"github.com/cmdblock/cbssh/internal/platform"
)

// StartOptions describes a foreground request to start one or more tunnels.
type StartOptions struct {
	ConfigPath  string
	StatePath   string
	LogDir      string
	HostName    string
	TunnelNames []string
	Timeout     time.Duration
}

// DaemonOptions describes the long-running process that owns live listeners.
type DaemonOptions struct {
	ConfigPath  string
	StatePath   string
	LogDir      string
	HostName    string
	TunnelNames []string
	RunID       string
}

type managedTunnel struct {
	Closer io.Closer
	Entry  model.TunnelRuntime
}

// dialer is the common subset shared by direct SSH clients and test doubles.
type dialer interface {
	Dial(network, addr string) (net.Conn, error)
	Listen(network, addr string) (net.Listener, error)
}

func (opts *StartOptions) normalize() {
	if opts.Timeout == 0 {
		opts.Timeout = 5 * time.Second
	}
	if opts.StatePath == "" {
		opts.StatePath = platform.DefaultStatePath()
	}
	if opts.ConfigPath == "" {
		opts.ConfigPath = platform.DefaultConfigPath()
	}
	if opts.LogDir == "" {
		opts.LogDir = platform.DefaultLogDir()
	}
	opts.LogDir = platform.ExpandPath(opts.LogDir)
}

func (opts *DaemonOptions) normalize() error {
	if opts.StatePath == "" {
		opts.StatePath = platform.DefaultStatePath()
	}
	if opts.ConfigPath == "" {
		opts.ConfigPath = platform.DefaultConfigPath()
	}
	if opts.LogDir == "" {
		opts.LogDir = daemonLogDir()
	}
	opts.LogDir = platform.ExpandPath(opts.LogDir)
	if opts.RunID == "" {
		return errors.New("run id is required")
	}
	return nil
}
