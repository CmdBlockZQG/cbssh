package sshclient

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/crypto/ssh"
	"golang.org/x/term"

	"github.com/cmdblock/cbssh/internal/model"
)

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
