package tunnel

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/cmdblock/cbssh/internal/model"
)

func startRuntimeTunnel(ctx context.Context, client dialer, tun model.Tunnel) (io.Closer, error) {
	switch tun.Type {
	case model.TunnelTypeLocal:
		return startLocal(ctx, client, tun)
	case model.TunnelTypeRemote:
		return startRemote(ctx, client, tun)
	case model.TunnelTypeDynamic:
		return startDynamic(ctx, client, tun)
	default:
		return nil, fmt.Errorf("unsupported tunnel type %q", tun.Type)
	}
}

func startLocal(ctx context.Context, client dialer, tun model.Tunnel) (io.Closer, error) {
	listener, err := net.Listen("tcp", tun.ListenAddress())
	if err != nil {
		return nil, err
	}
	runtime := newRuntimeTunnel(listener)
	go acceptLoop(ctx, runtime, func(local net.Conn) {
		remote, err := client.Dial("tcp", tun.TargetAddress())
		if err != nil {
			_ = local.Close()
			return
		}
		done := pipe(local, remote)
		<-done
	})
	return runtime, nil
}

func startRemote(ctx context.Context, client dialer, tun model.Tunnel) (io.Closer, error) {
	listener, err := client.Listen("tcp", tun.ListenAddress())
	if err != nil {
		return nil, err
	}
	runtime := newRuntimeTunnel(listener)
	go acceptLoop(ctx, runtime, func(remote net.Conn) {
		local, err := net.Dial("tcp", tun.TargetAddress())
		if err != nil {
			_ = remote.Close()
			return
		}
		done := pipe(local, remote)
		<-done
	})
	return runtime, nil
}

func startDynamic(ctx context.Context, client dialer, tun model.Tunnel) (io.Closer, error) {
	listener, err := net.Listen("tcp", tun.ListenAddress())
	if err != nil {
		return nil, err
	}
	runtime := newRuntimeTunnel(listener)
	go acceptLoop(ctx, runtime, func(local net.Conn) {
		handleSOCKS5(local, client)
	})
	return runtime, nil
}

type runtimeTunnel struct {
	listener net.Listener
	mu       sync.Mutex
	conns    map[net.Conn]struct{}
}

func newRuntimeTunnel(listener net.Listener) *runtimeTunnel {
	return &runtimeTunnel{
		listener: listener,
		conns:    map[net.Conn]struct{}{},
	}
}

func (t *runtimeTunnel) Close() error {
	err := t.listener.Close()
	t.mu.Lock()
	defer t.mu.Unlock()
	for conn := range t.conns {
		_ = conn.Close()
	}
	return err
}

func (t *runtimeTunnel) track(conn net.Conn) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.conns[conn] = struct{}{}
}

func (t *runtimeTunnel) untrack(conn net.Conn) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.conns, conn)
}

// acceptLoop tracks accepted connections so Close can tear down active streams.
func acceptLoop(ctx context.Context, tunnel *runtimeTunnel, handle func(net.Conn)) {
	for {
		conn, err := tunnel.listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				return
			}
		}
		tunnel.track(conn)
		go func() {
			defer tunnel.untrack(conn)
			handle(conn)
		}()
	}
}

// pipe copies bytes both ways and closes both connections when either side ends.
func pipe(a net.Conn, b net.Conn) <-chan struct{} {
	var once sync.Once
	done := make(chan struct{})
	closeBoth := func() {
		_ = a.Close()
		_ = b.Close()
		close(done)
	}
	go func() {
		_, _ = io.Copy(a, b)
		once.Do(closeBoth)
	}()
	go func() {
		_, _ = io.Copy(b, a)
		once.Do(closeBoth)
	}()
	return done
}
