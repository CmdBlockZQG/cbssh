package sshclient

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"

	"github.com/cmdblock/cbssh/internal/model"
	"github.com/cmdblock/cbssh/internal/platform"
)

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
