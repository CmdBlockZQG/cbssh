package filetransfer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
)

// CommandOutput contains the captured streams from one remote SSH exec command.
type CommandOutput struct {
	Stdout string
	Stderr string
}

// RunCommand opens one short-lived SSH exec channel on the existing target
// connection and captures stdout/stderr for display by the caller.
func (s *Session) RunCommand(ctx context.Context, command string) (CommandOutput, error) {
	if command == "" {
		return CommandOutput{}, errors.New("remote command is required")
	}
	if err := ctx.Err(); err != nil {
		return CommandOutput{}, err
	}
	target := s.chain.Target()
	if target == nil {
		return CommandOutput{}, errors.New("ssh target client is not available")
	}
	session, err := target.NewSession()
	if err != nil {
		return CommandOutput{}, fmt.Errorf("open remote command session: %w", err)
	}
	defer session.Close()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	done := make(chan error, 1)
	go func() {
		done <- session.Run(command)
	}()

	select {
	case err := <-done:
		return CommandOutput{Stdout: stdout.String(), Stderr: stderr.String()}, err
	case <-ctx.Done():
		_ = session.Close()
		err := <-done
		output := CommandOutput{Stdout: stdout.String(), Stderr: stderr.String()}
		if err != nil {
			return output, fmt.Errorf("%w: %v", ctx.Err(), err)
		}
		return output, ctx.Err()
	}
}
