package sshclient

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
	"golang.org/x/term"

	"github.com/cmdblock/cbssh/internal/model"
	"github.com/cmdblock/cbssh/internal/platform"
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

func RunInteractive(ctx context.Context, cfg model.Config, chain []model.Host) error {
	client, err := DialChain(ctx, cfg, chain)
	if err != nil {
		return err
	}
	defer client.Close()

	session, err := client.Target().NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return err
	}
	defer term.Restore(fd, oldState)

	width, height, err := term.GetSize(fd)
	if err != nil {
		width, height = 80, 24
	}
	termName := os.Getenv("TERM")
	if termName == "" {
		termName = "xterm-256color"
	}
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := session.RequestPty(termName, height, width, modes); err != nil {
		return err
	}

	session.Stdin = os.Stdin
	session.Stdout = os.Stdout
	session.Stderr = os.Stderr

	resizeDone := make(chan struct{})
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)
	go func() {
		defer close(resizeDone)
		for {
			select {
			case <-ctx.Done():
				return
			case _, ok := <-sigCh:
				if !ok {
					return
				}
				if width, height, err := term.GetSize(fd); err == nil {
					_ = session.WindowChange(height, width)
				}
			}
		}
	}()
	defer func() {
		signal.Stop(sigCh)
		close(sigCh)
		<-resizeDone
	}()

	if err := session.Shell(); err != nil {
		return err
	}
	return session.Wait()
}

func clientConfig(cfg model.Config, host model.Host) (*ssh.ClientConfig, error) {
	authMethods, err := authMethods(host)
	if err != nil {
		return nil, err
	}
	callback, err := hostKeyCallback(cfg.HostKeyCheck)
	if err != nil {
		return nil, err
	}
	return &ssh.ClientConfig{
		User:            host.User,
		Auth:            authMethods,
		HostKeyCallback: callback,
		Timeout:         15 * time.Second,
	}, nil
}

func authMethods(host model.Host) ([]ssh.AuthMethod, error) {
	switch host.Auth.Type {
	case model.AuthTypePassword:
		return []ssh.AuthMethod{ssh.Password(host.Auth.Password)}, nil
	case model.AuthTypeKey:
		var methods []ssh.AuthMethod
		if host.Auth.UseAgent {
			if method, err := agentAuthMethod(); err == nil {
				methods = append(methods, method)
			}
		}
		keyPath := platform.ExpandPath(host.Auth.KeyPath)
		key, err := os.ReadFile(keyPath)
		if err != nil {
			if len(methods) > 0 {
				return methods, nil
			}
			return nil, fmt.Errorf("read private key for host %q: %w", host.Name, err)
		}
		var signer ssh.Signer
		if host.Auth.Passphrase != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase(key, []byte(host.Auth.Passphrase))
		} else {
			signer, err = ssh.ParsePrivateKey(key)
		}
		if err != nil {
			return nil, fmt.Errorf("parse private key for host %q: %w", host.Name, err)
		}
		methods = append(methods, ssh.PublicKeys(signer))
		return methods, nil
	default:
		return nil, fmt.Errorf("unsupported auth type %q for host %q", host.Auth.Type, host.Name)
	}
}

func agentAuthMethod() (ssh.AuthMethod, error) {
	socket := os.Getenv("SSH_AUTH_SOCK")
	if socket == "" {
		return nil, errors.New("SSH_AUTH_SOCK is not set")
	}
	conn, err := net.Dial("unix", socket)
	if err != nil {
		return nil, err
	}
	return ssh.PublicKeysCallback(agent.NewClient(conn).Signers), nil
}

func hostKeyCallback(mode string) (ssh.HostKeyCallback, error) {
	switch mode {
	case "", "insecure":
		return ssh.InsecureIgnoreHostKey(), nil
	case "known_hosts", "known-hosts":
		path := filepath.Join(platform.ExpandPath("~"), ".ssh", "known_hosts")
		return knownhosts.New(path)
	default:
		return nil, fmt.Errorf("unsupported host_key_check %q", mode)
	}
}

func closeClients(clients []*ssh.Client) {
	for i := len(clients) - 1; i >= 0; i-- {
		_ = clients[i].Close()
	}
}
