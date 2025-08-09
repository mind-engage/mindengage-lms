package ocr

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"time"
)

type TesseractOCR struct {
	Lang    string
	Timeout time.Duration
}

func NewTesseractOCR() *TesseractOCR {
	return &TesseractOCR{Lang: "eng", Timeout: 20 * time.Second}
}

func (t *TesseractOCR) Extract(ctx context.Context, r io.Reader) (string, error) {
	f, err := os.CreateTemp("", "scan-*.img")
	if err != nil {
		return "", err
	}
	defer func() { f.Close(); os.Remove(f.Name()) }()
	if _, err := io.Copy(f, r); err != nil {
		return "", err
	}
	return t.exec(ctx, f.Name())
}

func (t *TesseractOCR) ExtractPath(ctx context.Context, path string) (string, error) {
	return t.exec(ctx, path)
}

func (t *TesseractOCR) exec(ctx context.Context, inPath string) (string, error) {
	if _, err := exec.LookPath("tesseract"); err != nil {
		return "", errors.New("tesseract not found in PATH")
	}
	args := []string{inPath, "stdout"}
	if t.Lang != "" {
		args = append(args, "-l", t.Lang)
	}
	if t.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, t.Timeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, "tesseract", args...)
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", errors.New(stderr.String())
	}
	return out.String(), nil
}
