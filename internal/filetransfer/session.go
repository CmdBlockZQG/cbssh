package filetransfer

import (
	"context"
	"errors"
	"fmt"

	"github.com/pkg/sftp"

	"github.com/cmdblock/cbssh/internal/config"
	"github.com/cmdblock/cbssh/internal/model"
	"github.com/cmdblock/cbssh/internal/sshclient"
)

// Session owns one SSH chain and one SFTP client. It can be reused by future TUI
// directory browsing so navigation does not reconnect for every directory read.
type Session struct {
	HostName string

	chain  *sshclient.ChainClient
	client *sftp.Client
}

// Dial resolves the configured jump chain and opens an SFTP session on the final host.
func Dial(ctx context.Context, cfg model.Config, hostName string) (*Session, error) {
	if hostName == "" {
		return nil, errors.New("host name is required")
	}
	chain, err := config.ResolveChain(cfg, hostName)
	if err != nil {
		return nil, err
	}
	sshChain, err := sshclient.DialChain(ctx, cfg, chain)
	if err != nil {
		return nil, err
	}
	client, err := sftp.NewClient(sshChain.Target())
	if err != nil {
		_ = sshChain.Close()
		return nil, fmt.Errorf("open sftp session on %s: %w", hostName, err)
	}
	return &Session{
		HostName: hostName,
		chain:    sshChain,
		client:   client,
	}, nil
}

// Close releases the SFTP client first, then unwinds the SSH jump chain.
func (s *Session) Close() error {
	if s == nil {
		return nil
	}
	var firstErr error
	if s.client != nil {
		if err := s.client.Close(); err != nil {
			firstErr = err
		}
	}
	if s.chain != nil {
		if err := s.chain.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Upload opens a short-lived session and uploads one file or directory.
func Upload(ctx context.Context, cfg model.Config, hostName, localPath, remotePath string, opts Options) (Result, error) {
	session, err := Dial(ctx, cfg, hostName)
	if err != nil {
		return Result{}, err
	}
	defer session.Close()
	return session.Upload(ctx, localPath, remotePath, opts)
}

// Download opens a short-lived session and downloads one file or directory.
func Download(ctx context.Context, cfg model.Config, hostName, remotePath, localPath string, opts Options) (Result, error) {
	session, err := Dial(ctx, cfg, hostName)
	if err != nil {
		return Result{}, err
	}
	defer session.Close()
	return session.Download(ctx, remotePath, localPath, opts)
}

// ListDir opens a short-lived session and lists one remote directory.
func ListDir(ctx context.Context, cfg model.Config, hostName, remotePath string) ([]Entry, error) {
	session, err := Dial(ctx, cfg, hostName)
	if err != nil {
		return nil, err
	}
	defer session.Close()
	return session.ListDir(remotePath)
}
