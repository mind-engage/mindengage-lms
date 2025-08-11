package parser

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Public manifest types in parser (no import of qti)
type Manifest struct {
	Resources []ManifestResource
}

type ManifestResource struct {
	Identifier string
	Href       string
	Type       string
	Files      []string
}

type imsManifest struct {
	XMLName   xml.Name      `xml:"manifest"`
	Resources []imsResource `xml:"resources>resource"`
}
type imsResource struct {
	Identifier string    `xml:"identifier,attr"`
	Href       string    `xml:"href,attr"`
	Type       string    `xml:"type,attr"`
	Files      []imsFile `xml:"file"`
}
type imsFile struct {
	Href string `xml:"href,attr"`
}

// Unzip to temp dir; return base dir.
func UnzipToTemp(r io.ReaderAt, size int64) (string, error) {
	tmp, err := os.MkdirTemp("", "qti-*")
	if err != nil {
		return "", err
	}
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return "", err
	}
	for _, f := range zr.File {
		dst := filepath.Join(tmp, f.Name)
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return "", err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return "", err
		}
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		defer rc.Close()
		out, err := os.Create(dst)
		if err != nil {
			return "", err
		}
		if _, err := io.Copy(out, rc); err != nil {
			_ = out.Close()
			return "", err
		}
		_ = out.Close()
	}
	return tmp, nil
}

func ParseManifest(base string) (Manifest, []string, error) {
	paths := []string{"imsmanifest.xml", "manifest.xml"}
	var mfPath string
	for _, p := range paths {
		if _, err := os.Stat(filepath.Join(base, p)); err == nil {
			mfPath = filepath.Join(base, p)
			break
		}
	}
	if mfPath == "" {
		return Manifest{}, nil, fmt.Errorf("imsmanifest.xml not found")
	}

	b, err := os.ReadFile(mfPath)
	if err != nil {
		return Manifest{}, nil, err
	}

	var mf imsManifest
	if err := xml.Unmarshal(b, &mf); err != nil {
		return Manifest{}, nil, err
	}

	var out Manifest
	var items []string
	for _, r := range mf.Resources {
		res := ManifestResource{
			Identifier: r.Identifier,
			Href:       r.Href,
			Type:       r.Type,
		}
		for _, f := range r.Files {
			res.Files = append(res.Files, f.Href)
		}
		out.Resources = append(out.Resources, res)
		if strings.HasSuffix(strings.ToLower(r.Href), ".xml") &&
			!strings.Contains(strings.ToLower(r.Href), "manifest") {
			items = append(items, r.Href)
		}
	}
	return out, items, nil
}
