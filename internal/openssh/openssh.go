package openssh

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/CmdBlockZQG/cbssh/internal/model"
)

type Master struct {
	Host          string
	SSHConfigPath string
	ControlPath   string
	LogPath       string
}

type Runner interface {
	StartMaster(context.Context, Master) error
	CheckMaster(context.Context, Master) (bool, error)
	Forward(context.Context, Master, string, string) error
	Cancel(context.Context, Master, string, string) error
	Exit(context.Context, Master) error
	ValidateHost(context.Context, string, string) error
}

type CommandRunner struct {
	Binary string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

func NewCommandRunner() *CommandRunner {
	return &CommandRunner{Binary: "ssh", Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr}
}

func (r *CommandRunner) StartMaster(ctx context.Context, master Master) error {
	args := make([]string, 0, 20)
	if master.SSHConfigPath != "" {
		args = append(args, "-F", master.SSHConfigPath)
	}
	args = append(args,
		"-f", "-M", "-N", "-T",
		"-S", master.ControlPath,
		"-E", master.LogPath,
		"-o", "ControlMaster=yes",
		"-o", "ControlPersist=no",
		"-o", "ClearAllForwardings=yes",
		"-o", "ExitOnForwardFailure=yes",
		"-o", "LogLevel=ERROR",
		master.Host,
	)
	if err := r.runBackground(ctx, args...); err != nil {
		return fmt.Errorf("start OpenSSH master for %s: %w (log: %s)", master.Host, err, master.LogPath)
	}
	return nil
}

func (r *CommandRunner) CheckMaster(ctx context.Context, master Master) (bool, error) {
	_, err := r.run(ctx, false, controlArgs(master, "check", "", "")...)
	if err == nil {
		return true, nil
	}
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return false, nil
	}
	return false, err
}

func (r *CommandRunner) Forward(ctx context.Context, master Master, flag, spec string) error {
	if _, err := r.run(ctx, false, controlArgs(master, "forward", flag, spec)...); err != nil {
		return fmt.Errorf("add %s %s on %s: %w", flag, spec, master.Host, err)
	}
	return nil
}

func (r *CommandRunner) Cancel(ctx context.Context, master Master, flag, spec string) error {
	if _, err := r.run(ctx, false, controlArgs(master, "cancel", flag, spec)...); err != nil {
		return fmt.Errorf("cancel %s %s on %s: %w", flag, spec, master.Host, err)
	}
	return nil
}

func (r *CommandRunner) Exit(ctx context.Context, master Master) error {
	if _, err := r.run(ctx, false, controlArgs(master, "exit", "", "")...); err != nil {
		return fmt.Errorf("exit OpenSSH master for %s: %w", master.Host, err)
	}
	return nil
}

func (r *CommandRunner) ValidateHost(ctx context.Context, host, sshConfigPath string) error {
	args := []string{"-G"}
	if sshConfigPath != "" {
		args = append(args, "-F", sshConfigPath)
	}
	args = append(args, host)
	if _, err := r.run(ctx, false, args...); err != nil {
		return fmt.Errorf("validate OpenSSH host %s: %w", host, err)
	}
	return nil
}

func controlArgs(master Master, operation, flag, spec string) []string {
	args := []string{"-F", os.DevNull, "-S", master.ControlPath, "-O", operation}
	if flag != "" {
		args = append(args, "-o", "ExitOnForwardFailure=yes", flag, spec)
	}
	return append(args, master.Host)
}

func (r *CommandRunner) run(ctx context.Context, interactive bool, args ...string) (string, error) {
	binary := r.Binary
	if binary == "" {
		binary = "ssh"
	}
	cmd := exec.CommandContext(ctx, binary, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if interactive {
		cmd.Stdin = r.Stdin
		cmd.Stdout = io.MultiWriter(writerOrDiscard(r.Stdout), &stdout)
		cmd.Stderr = io.MultiWriter(writerOrDiscard(r.Stderr), &stderr)
	} else {
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
	}
	err := cmd.Run()
	message := strings.TrimSpace(strings.TrimSpace(stdout.String()) + "\n" + strings.TrimSpace(stderr.String()))
	if err != nil && message != "" {
		err = fmt.Errorf("%w: %s", err, message)
	}
	return message, err
}

// runBackground starts ssh with direct file descriptors. A ProxyJump creates a
// helper ssh process that can outlive the foreground ssh -f process. If the
// streams are connected through os/exec pipes, that helper keeps the pipe open
// and Cmd.Run waits forever for EOF.
func (r *CommandRunner) runBackground(ctx context.Context, args ...string) error {
	binary := r.Binary
	if binary == "" {
		binary = "ssh"
	}
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Stdin = directReader(r.Stdin)
	cmd.Stdout = directWriter(r.Stdout)
	cmd.Stderr = directWriter(r.Stderr)
	return cmd.Run()
}

func directReader(reader io.Reader) io.Reader {
	// A non-file reader would make os/exec create a pipe and wait for it.
	if file, ok := reader.(*os.File); ok {
		return file
	}
	return nil
}

func directWriter(writer io.Writer) io.Writer {
	// nil makes os/exec use /dev/null directly instead of creating a pipe.
	if file, ok := writer.(*os.File); ok {
		return file
	}
	return nil
}

func writerOrDiscard(writer io.Writer) io.Writer {
	if writer == nil {
		return io.Discard
	}
	return writer
}

func ForwardArguments(tun model.Tunnel) (string, string, error) {
	spec, err := tun.ForwardSpec()
	if err != nil {
		return "", "", err
	}
	return tun.ForwardFlag(), spec, nil
}
