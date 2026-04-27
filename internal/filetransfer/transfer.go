package filetransfer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/cmdblock/cbssh/internal/platform"
)

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
	sortRemoteInfos(infos)

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
