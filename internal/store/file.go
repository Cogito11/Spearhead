package store

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"

	"spearhead/internal/protocol"
)

func (s *Store) FilePath(peer, id string) string { return s.p("files", peer, id) }

// StageFile streams to disk with constant memory and returns its content hash.
func (s *Store) StageFile(r io.Reader) (path string, size int64, hash string, err error) {
	f, err := os.CreateTemp(s.p("files"), ".upload-")
	if err != nil {
		return "", 0, "", err
	}
	path = f.Name()
	h := sha256.New()
	size, err = io.Copy(io.MultiWriter(f, h), io.LimitReader(r, protocol.MaxFileSize+1))
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if size > protocol.MaxFileSize {
		err = errors.New("files must be 256 MiB or smaller")
	}
	if err != nil {
		os.Remove(path)
		return "", 0, "", err
	}
	return path, size, hex.EncodeToString(h.Sum(nil)), nil
}

func (s *Store) CommitFile(staged, peer, id string) error {
	dst := s.FilePath(peer, id)
	if err := os.MkdirAll(filepath.Dir(dst), 0700); err != nil {
		return err
	}
	return os.Rename(staged, dst)
}
