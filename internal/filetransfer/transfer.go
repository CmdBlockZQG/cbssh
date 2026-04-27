package filetransfer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/pkg/sftp"

	"github.com/cmdblock/cbssh/internal/config"
	"github.com/cmdblock/cbssh/internal/model"
	"github.com/cmdblock/cbssh/internal/platform"
	"github.com/cmdblock/cbssh/internal/sshclient"
)

type Options struct {
	Recursive bool
	Force     bool
}

type Result struct {
	HostName    string
	LocalPath   string
	RemotePath  string
	Files       int
	Directories int
	Bytes       int64
}

type Entry struct {
	Name    string
	Path    string
	IsDir   bool
	Size    int64
	Mode    os.FileMode
	ModTime time.Time
}

type Session struct {
	HostName string

	chain  *sshclient.ChainClient
	client *sftp.Client
}

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

func Upload(ctx context.Context, cfg model.Config, hostName, localPath, remotePath string, opts Options) (Result, error) {
	session, err := Dial(ctx, cfg, hostName)
	if err != nil {
		return Result{}, err
	}
	defer session.Close()
	return session.Upload(ctx, localPath, remotePath, opts)
}

func Download(ctx context.Context, cfg model.Config, hostName, remotePath, localPath string, opts Options) (Result, error) {
	session, err := Dial(ctx, cfg, hostName)
	if err != nil {
		return Result{}, err
	}
	defer session.Close()
	return session.Download(ctx, remotePath, localPath, opts)
}

func ListDir(ctx context.Context, cfg model.Config, hostName, remotePath string) ([]Entry, error) {
	session, err := Dial(ctx, cfg, hostName)
	if err != nil {
		return nil, err
	}
	defer session.Close()
	return session.ListDir(remotePath)
}

func (s *Session) Upload(ctx context.Context, localPath, remotePath string, opts Options) (Result, error) {
	if localPath == "" {
		return Result{}, errors.New("local path is required")
	}
	localPath = platform.ExpandPath(localPath)
	info, err := os.Stat(localPath)
	if err != nil {
		return Result{}, fmt.Errorf("stat local path %s: %w", localPath, err)
	}

	result := Result{HostName: s.HostName, LocalPath: localPath}
	switch {
	case info.IsDir():
		if !opts.Recursive {
			return Result{}, fmt.Errorf("local path %s is a directory; use --recursive to upload directories", localPath)
		}
		dest, err := s.uploadDirDestination(localPath, remotePath)
		if err != nil {
			return Result{}, err
		}
		result.RemotePath = dest
		if err := s.uploadDir(ctx, localPath, dest, opts, &result); err != nil {
			return Result{}, err
		}
	case info.Mode().IsRegular():
		dest, err := s.uploadFileDestination(localPath, remotePath)
		if err != nil {
			return Result{}, err
		}
		result.RemotePath = dest
		if err := s.uploadFile(localPath, dest, info, opts, &result); err != nil {
			return Result{}, err
		}
	default:
		return Result{}, fmt.Errorf("unsupported local file type %s", localPath)
	}
	return result, nil
}

func (s *Session) Download(ctx context.Context, remotePath, localPath string, opts Options) (Result, error) {
	if remotePath == "" {
		return Result{}, errors.New("remote path is required")
	}
	src, err := s.normalizeRemotePath(remotePath)
	if err != nil {
		return Result{}, err
	}
	info, err := s.client.Stat(src)
	if err != nil {
		return Result{}, fmt.Errorf("stat remote path %s: %w", src, err)
	}

	result := Result{HostName: s.HostName, RemotePath: src}
	switch {
	case info.IsDir():
		if !opts.Recursive {
			return Result{}, fmt.Errorf("remote path %s is a directory; use --recursive to download directories", src)
		}
		dest := downloadDirDestination(src, localPath)
		if dest == "" {
			return Result{}, fmt.Errorf("remote path %s has no directory name; specify a local path", src)
		}
		result.LocalPath = dest
		if err := s.downloadDir(ctx, src, dest, opts, &result); err != nil {
			return Result{}, err
		}
	case info.Mode().IsRegular():
		dest, err := downloadFileDestination(src, localPath)
		if err != nil {
			return Result{}, err
		}
		result.LocalPath = dest
		if err := s.downloadFile(src, dest, info, opts, &result); err != nil {
			return Result{}, err
		}
	default:
		return Result{}, fmt.Errorf("unsupported remote file type %s", src)
	}
	return result, nil
}

func (s *Session) ListDir(remotePath string) ([]Entry, error) {
	dir, err := s.normalizeRemotePath(defaultRemotePath(remotePath))
	if err != nil {
		return nil, err
	}
	infos, err := s.client.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read remote directory %s: %w", dir, err)
	}
	sort.SliceStable(infos, func(i, j int) bool {
		if infos[i].IsDir() != infos[j].IsDir() {
			return infos[i].IsDir()
		}
		return infos[i].Name() < infos[j].Name()
	})

	entries := make([]Entry, 0, len(infos))
	for _, info := range infos {
		entries = append(entries, Entry{
			Name:    info.Name(),
			Path:    remoteJoin(dir, info.Name()),
			IsDir:   info.IsDir(),
			Size:    info.Size(),
			Mode:    info.Mode(),
			ModTime: info.ModTime(),
		})
	}
	return entries, nil
}

func (s *Session) uploadDirDestination(localPath, remotePath string) (string, error) {
	if remotePath == "" {
		return s.normalizeRemotePath(filepath.Base(localPath))
	}
	return s.normalizeRemotePath(remotePath)
}

func (s *Session) uploadFileDestination(localPath, remotePath string) (string, error) {
	info, err := os.Stat(localPath)
	if err != nil {
		return "", fmt.Errorf("stat local path %s: %w", localPath, err)
	}
	if remotePath == "" {
		return s.normalizeRemotePath(info.Name())
	}
	dest, err := s.normalizeRemotePath(remotePath)
	if err != nil {
		return "", err
	}
	if hasRemoteTrailingSlash(remotePath) {
		return remoteJoin(dest, info.Name()), nil
	}
	remoteInfo, err := s.client.Stat(dest)
	if err == nil {
		if remoteInfo.IsDir() {
			return remoteJoin(dest, info.Name()), nil
		}
		return dest, nil
	}
	if !isNotExist(err) {
		return "", fmt.Errorf("stat remote path %s: %w", dest, err)
	}
	return dest, nil
}

func (s *Session) uploadDir(ctx context.Context, localRoot, remoteRoot string, opts Options, result *Result) error {
	if err := ensureRemoteDirectory(s.client, remoteRoot); err != nil {
		return err
	}
	result.Directories++

	return filepath.WalkDir(localRoot, func(localPath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if localPath == localRoot {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}

		rel, err := filepath.Rel(localRoot, localPath)
		if err != nil {
			return err
		}
		remotePath := remoteJoinRel(remoteRoot, rel)
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("stat local path %s: %w", localPath, err)
		}

		switch {
		case entry.IsDir():
			if err := ensureRemoteDirectory(s.client, remotePath); err != nil {
				return err
			}
			result.Directories++
		case info.Mode().IsRegular():
			if err := s.uploadFile(localPath, remotePath, info, opts, result); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported local file type %s", localPath)
		}
		return nil
	})
}

func (s *Session) uploadFile(localPath, remotePath string, info os.FileInfo, opts Options, result *Result) error {
	if err := ensureRemoteFileWritable(s.client, remotePath, opts.Force); err != nil {
		return err
	}
	if err := ensureRemoteParent(s.client, remotePath); err != nil {
		return err
	}

	localFile, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("open local file %s: %w", localPath, err)
	}
	defer localFile.Close()

	flags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	if !opts.Force {
		flags |= os.O_EXCL
	}
	remoteFile, err := s.client.OpenFile(remotePath, flags)
	if err != nil {
		if !opts.Force && os.IsExist(err) {
			return fmt.Errorf("remote file %s already exists; use --force to overwrite", remotePath)
		}
		return fmt.Errorf("open remote file %s: %w", remotePath, err)
	}
	bytes, copyErr := io.Copy(remoteFile, localFile)
	closeErr := remoteFile.Close()
	if copyErr != nil {
		return fmt.Errorf("upload %s to %s: %w", localPath, remotePath, copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close remote file %s: %w", remotePath, closeErr)
	}
	if mode := info.Mode().Perm(); mode != 0 {
		if err := s.client.Chmod(remotePath, mode); err != nil {
			return fmt.Errorf("chmod remote file %s: %w", remotePath, err)
		}
	}

	result.Files++
	result.Bytes += bytes
	return nil
}

func (s *Session) downloadDir(ctx context.Context, remoteRoot, localRoot string, opts Options, result *Result) error {
	if err := ensureLocalDirectory(localRoot); err != nil {
		return err
	}
	result.Directories++

	infos, err := s.client.ReadDir(remoteRoot)
	if err != nil {
		return fmt.Errorf("read remote directory %s: %w", remoteRoot, err)
	}
	sort.SliceStable(infos, func(i, j int) bool {
		if infos[i].IsDir() != infos[j].IsDir() {
			return infos[i].IsDir()
		}
		return infos[i].Name() < infos[j].Name()
	})

	for _, info := range infos {
		if err := ctx.Err(); err != nil {
			return err
		}
		remotePath := remoteJoin(remoteRoot, info.Name())
		localPath := filepath.Join(localRoot, info.Name())
		switch {
		case info.IsDir():
			if err := s.downloadDir(ctx, remotePath, localPath, opts, result); err != nil {
				return err
			}
		case info.Mode().IsRegular():
			if err := s.downloadFile(remotePath, localPath, info, opts, result); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported remote file type %s", remotePath)
		}
	}
	return nil
}

func (s *Session) downloadFile(remotePath, localPath string, info os.FileInfo, opts Options, result *Result) error {
	if err := ensureLocalFileWritable(localPath, opts.Force); err != nil {
		return err
	}
	if err := ensureLocalParent(localPath); err != nil {
		return err
	}

	remoteFile, err := s.client.Open(remotePath)
	if err != nil {
		return fmt.Errorf("open remote file %s: %w", remotePath, err)
	}
	defer remoteFile.Close()

	mode := info.Mode().Perm()
	if mode == 0 {
		mode = 0o644
	}
	flags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	if !opts.Force {
		flags |= os.O_EXCL
	}
	localFile, err := os.OpenFile(localPath, flags, mode)
	if err != nil {
		if !opts.Force && os.IsExist(err) {
			return fmt.Errorf("local file %s already exists; use --force to overwrite", localPath)
		}
		return fmt.Errorf("open local file %s: %w", localPath, err)
	}
	bytes, copyErr := io.Copy(localFile, remoteFile)
	closeErr := localFile.Close()
	if copyErr != nil {
		return fmt.Errorf("download %s to %s: %w", remotePath, localPath, copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close local file %s: %w", localPath, closeErr)
	}
	if err := os.Chmod(localPath, mode); err != nil {
		return fmt.Errorf("chmod local file %s: %w", localPath, err)
	}

	result.Files++
	result.Bytes += bytes
	return nil
}

func (s *Session) normalizeRemotePath(remotePath string) (string, error) {
	if remotePath == "" {
		return s.remoteInitialDirectory()
	}
	if remotePath == "~" || strings.HasPrefix(remotePath, "~/") {
		wd, err := s.remoteInitialDirectory()
		if err != nil {
			return "", err
		}
		if remotePath == "~" {
			return wd, nil
		}
		return remoteJoin(wd, strings.TrimPrefix(remotePath, "~/")), nil
	}
	if !path.IsAbs(remotePath) {
		wd, err := s.remoteInitialDirectory()
		if err != nil {
			return "", err
		}
		return remoteJoin(wd, remotePath), nil
	}
	return path.Clean(remotePath), nil
}

func (s *Session) remoteInitialDirectory() (string, error) {
	wd, err := s.client.Getwd()
	if err != nil {
		return "", fmt.Errorf("get remote initial directory: %w", err)
	}
	return path.Clean(wd), nil
}

func defaultRemotePath(remotePath string) string {
	if remotePath == "" {
		return "."
	}
	return remotePath
}

func downloadDirDestination(remotePath, localPath string) string {
	if localPath == "" {
		return platform.ExpandPath(remoteBase(remotePath))
	}
	return platform.ExpandPath(localPath)
}

func downloadFileDestination(remotePath, localPath string) (string, error) {
	base := remoteBase(remotePath)
	if base == "" {
		return "", fmt.Errorf("remote path %s has no file name", remotePath)
	}
	if localPath == "" {
		return platform.ExpandPath(base), nil
	}
	dest := platform.ExpandPath(localPath)
	if hasLocalTrailingSeparator(localPath) {
		return filepath.Join(dest, base), nil
	}
	info, err := os.Stat(dest)
	if err == nil {
		if info.IsDir() {
			return filepath.Join(dest, base), nil
		}
		return dest, nil
	}
	if !os.IsNotExist(err) {
		return "", fmt.Errorf("stat local path %s: %w", dest, err)
	}
	return dest, nil
}

func remoteBase(remotePath string) string {
	cleaned := path.Clean(remotePath)
	base := path.Base(cleaned)
	if base == "." || base == "/" {
		return ""
	}
	return base
}

func remoteJoin(elem ...string) string {
	return path.Join(elem...)
}

func remoteJoinRel(root, rel string) string {
	remoteRel := filepath.ToSlash(rel)
	if remoteRel == "." {
		return root
	}
	return remoteJoin(root, remoteRel)
}

func hasRemoteTrailingSlash(remotePath string) bool {
	return strings.HasSuffix(remotePath, "/")
}

func hasLocalTrailingSeparator(localPath string) bool {
	return strings.HasSuffix(localPath, string(os.PathSeparator)) || strings.HasSuffix(localPath, "/")
}

func ensureRemoteDirectory(client *sftp.Client, remotePath string) error {
	info, err := client.Stat(remotePath)
	if err == nil {
		if !info.IsDir() {
			return fmt.Errorf("remote path %s exists and is not a directory", remotePath)
		}
		return nil
	}
	if !isNotExist(err) {
		return fmt.Errorf("stat remote directory %s: %w", remotePath, err)
	}
	if err := client.MkdirAll(remotePath); err != nil {
		return fmt.Errorf("create remote directory %s: %w", remotePath, err)
	}
	return nil
}

func ensureRemoteParent(client *sftp.Client, remotePath string) error {
	parent := path.Dir(remotePath)
	if parent == "." || parent == "/" {
		return nil
	}
	return ensureRemoteDirectory(client, parent)
}

func ensureRemoteFileWritable(client *sftp.Client, remotePath string, force bool) error {
	info, err := client.Stat(remotePath)
	if err == nil {
		if info.IsDir() {
			return fmt.Errorf("remote path %s is a directory", remotePath)
		}
		if !force {
			return fmt.Errorf("remote file %s already exists; use --force to overwrite", remotePath)
		}
		return nil
	}
	if isNotExist(err) {
		return nil
	}
	return fmt.Errorf("stat remote file %s: %w", remotePath, err)
}

func ensureLocalDirectory(localPath string) error {
	info, err := os.Stat(localPath)
	if err == nil {
		if !info.IsDir() {
			return fmt.Errorf("local path %s exists and is not a directory", localPath)
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return fmt.Errorf("stat local directory %s: %w", localPath, err)
	}
	if err := os.MkdirAll(localPath, 0o755); err != nil {
		return fmt.Errorf("create local directory %s: %w", localPath, err)
	}
	return nil
}

func ensureLocalParent(localPath string) error {
	parent := filepath.Dir(localPath)
	if parent == "." || parent == string(os.PathSeparator) {
		return nil
	}
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("create local directory %s: %w", parent, err)
	}
	return nil
}

func ensureLocalFileWritable(localPath string, force bool) error {
	info, err := os.Stat(localPath)
	if err == nil {
		if info.IsDir() {
			return fmt.Errorf("local path %s is a directory", localPath)
		}
		if !force {
			return fmt.Errorf("local file %s already exists; use --force to overwrite", localPath)
		}
		return nil
	}
	if os.IsNotExist(err) {
		return nil
	}
	return fmt.Errorf("stat local file %s: %w", localPath, err)
}

func isNotExist(err error) bool {
	if os.IsNotExist(err) {
		return true
	}
	var status *sftp.StatusError
	return errors.As(err, &status) && status.FxCode() == sftp.ErrSSHFxNoSuchFile
}
