package storage

import (
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
)

type FSStore struct{ base string }

func NewFSStore(base string) (*FSStore, error) {
	if base == "" {
		base = "./data"
	}
	if err := os.MkdirAll(base, 0o755); err != nil {
		return nil, err
	}
	return &FSStore{base: base}, nil
}

func (s *FSStore) Put(key string, r io.Reader) (string, error) {
	if key == "" {
		return "", errors.New("empty key")
	}
	dst := filepath.Join(s.base, filepath.Clean(key))
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
	return key, nil
}

func (s *FSStore) Get(key string) (io.ReadCloser, error) {
	return os.Open(filepath.Join(s.base, filepath.Clean(key)))
}

func (s *FSStore) SignedURL(key string) (string, error) {
	u := url.URL{Scheme: "file", Path: filepath.Join(s.base, key)}
	return u.String(), nil
}
