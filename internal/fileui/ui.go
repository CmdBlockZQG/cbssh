package fileui

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/cmdblock/cbssh/internal/filetransfer"
	"github.com/cmdblock/cbssh/internal/model"
	"github.com/cmdblock/cbssh/internal/platform"
)

const (
	styleBold  = "\033[1m"
	styleGreen = "\033[32m"
	styleRed   = "\033[31m"
	styleCyan  = "\033[36m"
	styleDim   = "\033[2m"
	styleReset = "\033[0m"
)

var errCanceled = errors.New("canceled")

type ui struct {
	hostName string
	session  *filetransfer.Session
	reader   *bufio.Reader
	cwd      string
	entries  []filetransfer.Entry
	visible  []filetransfer.Entry
	showDot  bool
	message  string
}

type command struct {
	action string
	args   []string
}

func Run(ctx context.Context, cfg model.Config, hostName string) error {
	session, err := filetransfer.Dial(ctx, cfg, hostName)
	if err != nil {
		return err
	}
	defer session.Close()

	cwd, err := session.ResolveRemotePath("", "")
	if err != nil {
		return err
	}
	app := &ui{
		hostName: hostName,
		session:  session,
		reader:   bufio.NewReader(os.Stdin),
		cwd:      cwd,
	}
	if err := app.refresh(); err != nil {
		return err
	}
	return app.loop(ctx)
}

func (u *ui) loop(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		clearScreen()
		u.render()
		raw, err := u.readLine("Action")
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		cmd := parseCommand(raw)
		if cmd.action == "" {
			continue
		}
		if err := u.dispatch(ctx, cmd); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			if errors.Is(err, errCanceled) {
				continue
			}
			u.message = styleRed + styleBold + err.Error() + styleReset
		}
		u.printMessage()
	}
}

func (u *ui) dispatch(ctx context.Context, cmd command) error {
	switch cmd.action {
	case "q":
		return io.EOF
	case "?":
		u.printHelp()
		u.waitEnter()
	case "r":
		return u.refresh()
	case "h":
		u.showDot = !u.showDot
		u.applyEntryFilter()
	case "c", "cd":
		if err := requireMaxArgs(cmd, 1); err != nil {
			return err
		}
		return u.changeDir(firstArg(cmd.args))
	case "u":
		if err := requireMaxArgs(cmd, 2); err != nil {
			return err
		}
		return u.upload(ctx, cmd.args)
	case "d":
		if err := requireMaxArgs(cmd, 2); err != nil {
			return err
		}
		return u.download(ctx, cmd.args)
	default:
		return fmt.Errorf("unknown command %q, press ? for help", cmd.action)
	}
	return nil
}

func (u *ui) refresh() error {
	entries, err := u.session.ListDir(u.cwd)
	if err != nil {
		return err
	}
	u.entries = entries
	u.applyEntryFilter()
	return nil
}

func (u *ui) applyEntryFilter() {
	u.visible = u.visible[:0]
	for _, entry := range u.entries {
		if !u.showDot && isHiddenName(entry.Name) {
			continue
		}
		u.visible = append(u.visible, entry)
	}
}

func (u *ui) changeDir(selector string) error {
	if selector == "" {
		selector = u.prompt("Directory number or path", "")
	}
	if selector == "" {
		return errCanceled
	}
	var next string
	switch selector {
	case "0", "..":
		next = path.Dir(u.cwd)
	default:
		resolved, err := u.resolveRemoteSelector(selector)
		if err != nil {
			return err
		}
		next = resolved
	}
	info, err := u.session.Stat(next)
	if err != nil {
		return fmt.Errorf("stat remote path %s: %w", next, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("remote path %s is not a directory", next)
	}
	u.cwd = next
	return u.refresh()
}

func (u *ui) upload(ctx context.Context, args []string) error {
	localPath := argAt(args, 0)
	if localPath == "" {
		localPath = u.prompt("Local path to upload", "")
	}
	if localPath == "" {
		return errCanceled
	}
	localPath = platform.ExpandPath(localPath)
	info, err := os.Stat(localPath)
	if err != nil {
		return fmt.Errorf("stat local path %s: %w", localPath, err)
	}
	opts := filetransfer.Options{}
	if info.IsDir() {
		if !u.confirm("Upload directory recursively", true) {
			return errCanceled
		}
		opts.Recursive = true
	}
	remoteInput := argAt(args, 1)
	if remoteInput == "" {
		remoteInput = u.prompt("Remote path relative to current directory", ".")
	}
	remoteDest, err := u.uploadDestination(localPath, remoteInput)
	if err != nil {
		return err
	}
	force, err := u.confirmRemoteOverwrite(remoteDest)
	if err != nil {
		return err
	}
	opts.Force = force
	result, err := u.session.Upload(ctx, localPath, remoteDest, opts)
	if err != nil {
		return err
	}
	u.message = styleGreen + formatTransferResult("Uploaded", result) + styleReset
	return u.refresh()
}

func (u *ui) download(ctx context.Context, args []string) error {
	selector := argAt(args, 0)
	if selector == "" {
		selector = u.prompt("Remote path or number", ".")
	}
	if selector == "" {
		return errCanceled
	}
	remoteSrc, err := u.resolveRemoteSelector(selector)
	if err != nil {
		return err
	}
	info, err := u.session.Stat(remoteSrc)
	if err != nil {
		return fmt.Errorf("stat remote path %s: %w", remoteSrc, err)
	}
	opts := filetransfer.Options{}
	if info.IsDir() {
		if !u.confirm("Download directory recursively", true) {
			return errCanceled
		}
		opts.Recursive = true
	}
	localInput := argAt(args, 1)
	if localInput == "" {
		localInput = u.prompt("Local destination", ".")
	}
	localDest := downloadDestination(remoteSrc, localInput)
	force, err := u.confirmLocalOverwrite(localDest)
	if err != nil {
		return err
	}
	opts.Force = force
	result, err := u.session.Download(ctx, remoteSrc, localDest, opts)
	if err != nil {
		return err
	}
	u.message = styleGreen + formatTransferResult("Downloaded", result) + styleReset
	return u.refresh()
}

func (u *ui) uploadDestination(localPath string, remoteInput string) (string, error) {
	base := filepath.Base(filepath.Clean(localPath))
	remoteInput = strings.TrimSpace(remoteInput)
	if remoteInput == "" || remoteInput == "." || strings.HasSuffix(remoteInput, "/") {
		dir, err := u.session.ResolveRemotePath(u.cwd, remoteInput)
		if err != nil {
			return "", err
		}
		return path.Join(dir, base), nil
	}
	target, err := u.session.ResolveRemotePath(u.cwd, remoteInput)
	if err != nil {
		return "", err
	}
	if info, err := u.session.Stat(target); err == nil && info.IsDir() {
		return path.Join(target, base), nil
	}
	return target, nil
}

func downloadDestination(remoteSrc string, localInput string) string {
	base := path.Base(remoteSrc)
	if base == "." || base == "/" {
		base = "remote"
	}
	localInput = strings.TrimSpace(localInput)
	if localInput == "" || localInput == "." || hasLocalTrailingSeparator(localInput) {
		root := platform.ExpandPath(localInput)
		if root == "" {
			root = "."
		}
		return filepath.Join(root, base)
	}
	dest := platform.ExpandPath(localInput)
	if info, err := os.Stat(dest); err == nil && info.IsDir() {
		return filepath.Join(dest, base)
	}
	return dest
}

func (u *ui) confirmRemoteOverwrite(remotePath string) (bool, error) {
	_, err := u.session.Stat(remotePath)
	if err != nil {
		if filetransfer.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("stat remote path %s: %w", remotePath, err)
	}
	if !u.confirm("Remote path exists; overwrite", false) {
		return false, errCanceled
	}
	return true, nil
}

func (u *ui) confirmLocalOverwrite(localPath string) (bool, error) {
	if _, err := os.Stat(localPath); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("stat local path %s: %w", localPath, err)
	}
	if !u.confirm("Local path exists; overwrite", false) {
		return false, errCanceled
	}
	return true, nil
}

func hasLocalTrailingSeparator(value string) bool {
	return strings.HasSuffix(value, string(os.PathSeparator)) || strings.HasSuffix(value, "/")
}

func (u *ui) resolveRemoteSelector(selector string) (string, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return "", errCanceled
	}
	if selector == "0" || selector == ".." {
		return path.Dir(u.cwd), nil
	}
	if index, ok := parseEntryNumber(selector); ok {
		if index < 1 || index > len(u.visible) {
			return "", fmt.Errorf("remote entry number %d out of range", index)
		}
		return u.visible[index-1].Path, nil
	}
	return u.session.ResolveRemotePath(u.cwd, selector)
}

func isHiddenName(name string) bool {
	return strings.HasPrefix(name, ".")
}

func requireMaxArgs(cmd command, max int) error {
	if len(cmd.args) > max {
		return fmt.Errorf("%s accepts at most %d argument(s)", cmd.action, max)
	}
	return nil
}

func argAt(args []string, index int) string {
	if index >= len(args) {
		return ""
	}
	return args[index]
}
