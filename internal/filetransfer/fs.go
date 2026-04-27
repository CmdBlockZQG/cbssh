package filetransfer

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/pkg/sftp"
)

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
