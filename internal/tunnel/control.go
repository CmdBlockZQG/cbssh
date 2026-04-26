package tunnel

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/cmdblock/cbssh/internal/model"
	"github.com/cmdblock/cbssh/internal/platform"
)

const controlTimeout = 5 * time.Second

type controlRequest struct {
	Op         string         `json:"op"`
	ProcessKey string         `json:"process_key"`
	Tunnels    []string       `json:"tunnels"`
	TunnelDefs []model.Tunnel `json:"tunnel_defs,omitempty"`
}

type controlResponse struct {
	Entries []model.TunnelRuntime `json:"entries,omitempty"`
	Error   string                `json:"error,omitempty"`
}

type daemonCommand struct {
	Request  controlRequest
	Response chan controlResponse
}

func controlSocketPath(statePath string, runID string) string {
	statePath = platform.ExpandPath(statePath)
	if statePath == "" {
		statePath = platform.DefaultStatePath()
	}
	sum := sha256.Sum256([]byte(runID))
	name := hex.EncodeToString(sum[:16]) + ".sock"
	return filepath.Join(filepath.Dir(statePath), "sockets", name)
}

func listenControl(path string) (net.Listener, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	_ = os.Remove(path)
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = listener.Close()
		_ = os.Remove(path)
		return nil, err
	}
	return listener, nil
}

func serveControl(ctx context.Context, listener net.Listener, commands chan<- daemonCommand) {
	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		go handleControlConn(ctx, conn, commands)
	}
}

func handleControlConn(ctx context.Context, conn net.Conn, commands chan<- daemonCommand) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(controlTimeout))
	var req controlRequest
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		_ = json.NewEncoder(conn).Encode(controlResponse{Error: err.Error()})
		return
	}
	respCh := make(chan controlResponse, 1)
	select {
	case commands <- daemonCommand{Request: req, Response: respCh}:
	case <-ctx.Done():
		_ = json.NewEncoder(conn).Encode(controlResponse{Error: "daemon is shutting down"})
		return
	}
	select {
	case resp := <-respCh:
		_ = json.NewEncoder(conn).Encode(resp)
	case <-ctx.Done():
		_ = json.NewEncoder(conn).Encode(controlResponse{Error: "daemon is shutting down"})
	}
}

func sendControl(ctx context.Context, entry model.TunnelRuntime, req controlRequest) (controlResponse, error) {
	if entry.ControlPath == "" {
		return controlResponse{}, errors.New("control path is unavailable")
	}
	if !platform.ProcessMatches(entry.PID, entry.ProcessKey) {
		return controlResponse{}, fmt.Errorf("daemon process %d no longer matches state", entry.PID)
	}
	req.ProcessKey = entry.ProcessKey
	dialer := net.Dialer{Timeout: controlTimeout}
	conn, err := dialer.DialContext(ctx, "unix", entry.ControlPath)
	if err != nil {
		return controlResponse{}, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(controlTimeout))
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return controlResponse{}, err
	}
	var resp controlResponse
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return controlResponse{}, err
	}
	if resp.Error != "" {
		return controlResponse{}, errors.New(resp.Error)
	}
	return resp, nil
}
