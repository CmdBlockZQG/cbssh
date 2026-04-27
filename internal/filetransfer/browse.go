package filetransfer

import (
	"fmt"
	"os"
	"sort"
)

func (s *Session) Stat(remotePath string) (os.FileInfo, error) {
	target, err := s.normalizeRemotePath(remotePath)
	if err != nil {
		return nil, err
	}
	return s.client.Stat(target)
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
	sortRemoteInfos(infos)

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

func sortRemoteInfos(infos []os.FileInfo) {
	sort.SliceStable(infos, func(i, j int) bool {
		if infos[i].IsDir() != infos[j].IsDir() {
			return infos[i].IsDir()
		}
		return infos[i].Name() < infos[j].Name()
	})
}
