package sshclient

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/cmdblock/cbssh/internal/model"
)

type ChainClient struct {
	Clients []*ssh.Client
	Chain   []model.Host
}

func (c *ChainClient) Target() *ssh.Client {
	if c == nil || len(c.Clients) == 0 {
		return nil
	}
	return c.Clients[len(c.Clients)-1]
}

func (c *ChainClient) Close() error {
	var firstErr error
	for i := len(c.Clients) - 1; i >= 0; i-- {
		if err := c.Clients[i].Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// DialChain connects through configured jump hosts and returns the final target
// client. Callers such as tunnels and SFTP reuse this to share one jump-chain
// implementation.
func DialChain(ctx context.Context, cfg model.Config, chain []model.Host) (*ChainClient, error) {
	if len(chain) == 0 {
		return nil, errors.New("empty SSH chain")
	}

	clients := make([]*ssh.Client, 0, len(chain))
	var previous *ssh.Client
	for _, host := range chain {
		clientConfig, err := clientConfig(cfg, host)
		if err != nil {
			closeClients(clients)
			return nil, err
		}

		addr := host.Address()
		var conn net.Conn
		if previous == nil {
			dialer := net.Dialer{Timeout: 15 * time.Second}
			conn, err = dialer.DialContext(ctx, "tcp", addr)
		} else {
			conn, err = previous.DialContext(ctx, "tcp", addr)
		}
		if err != nil {
			closeClients(clients)
			return nil, fmt.Errorf("dial %s: %w", host.Name, err)
		}

		sshConn, chans, reqs, err := ssh.NewClientConn(conn, addr, clientConfig)
		if err != nil {
			_ = conn.Close()
			closeClients(clients)
			return nil, fmt.Errorf("handshake %s: %w", host.Name, err)
		}
		client := ssh.NewClient(sshConn, chans, reqs)
		clients = append(clients, client)
		previous = client
	}

	return &ChainClient{Clients: clients, Chain: chain}, nil
}

func closeClients(clients []*ssh.Client) {
	for i := len(clients) - 1; i >= 0; i-- {
		_ = clients[i].Close()
	}
}
