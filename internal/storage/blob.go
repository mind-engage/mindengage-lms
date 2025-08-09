package storage

import "io"

type BlobStore interface {
	Put(key string, r io.Reader) (string, error) // returns canonical key
	Get(key string) (io.ReadCloser, error)
	SignedURL(key string) (string, error) // fs returns "file://..." for dev
}
