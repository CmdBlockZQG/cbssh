package filetransfer

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/cmdblock/cbssh/internal/platform"
)

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

// ResolveRemotePath resolves value against base using SFTP path semantics. It is
// intended for stateful callers such as the file TUI, where relative paths should
// follow the current remote directory instead of the session's initial directory.
func (s *Session) ResolveRemotePath(base string, value string) (string, error) {
	basePath, err := s.normalizeRemotePath(base)
	if err != nil {
		return "", err
	}
	value = strings.TrimSpace(value)
	switch {
	case value == "" || value == ".":
		return basePath, nil
	case value == "~" || strings.HasPrefix(value, "~/") || path.IsAbs(value):
		return s.normalizeRemotePath(value)
	default:
		return remoteJoin(basePath, value), nil
	}
}

// normalizeRemotePath converts every user-provided remote path to a POSIX path
// under the target SFTP server. Unquoted ~/path never reaches this function:
// the user's local shell expands it before cbssh starts.
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
