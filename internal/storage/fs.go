// internal/storage/fs.go
package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type FSStore struct{ base string }

func NewFSStore(base string) (*FSStore, error) {
	if strings.TrimSpace(base) == "" {
		return nil, fmt.Errorf("blob base path is empty; set BLOB_BASE_PATH or Config.BlobBasePath")
	}
	abs, err := filepath.Abs(base)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, err
	}
	return &FSStore{base: abs}, nil
}

func (s *FSStore) Put(key string, r io.Reader) (string, error) {
	key = strings.TrimPrefix(key, "/")
	clean := filepath.Clean(key)
	dst := filepath.Join(s.base, clean)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", err
	}
	f, err := os.Create(dst)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := io.Copy(f, r); err != nil {
		return "", err
	}
	return clean, nil
}

func (s *FSStore) Get(key string) (io.ReadCloser, error) {
	key = strings.TrimPrefix(key, "/")
	clean := filepath.Clean(key)
	return os.Open(filepath.Join(s.base, clean))
}

func (s *FSStore) SignedURL(key string) (string, error) {
	key = strings.TrimPrefix(key, "/")
	return "file://" + filepath.Join(s.base, filepath.Clean(key)), nil
}
