package main

import (
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

type FileStorage struct {
	BasePath string
}

func NewFileStorage(basePath string) (*FileStorage, error) {
	if err := os.MkdirAll(basePath, 0o750); err != nil {
		return nil, fmt.Errorf("create attachments dir: %w", err)
	}
	return &FileStorage{BasePath: basePath}, nil
}

func (s *FileStorage) Save(originalName string, r io.Reader) (string, string, error) {
	base := filepath.Base(originalName)
	ext := filepath.Ext(base)
	random := rand.Text()
	name := fmt.Sprintf("%d_%s%s", time.Now().UnixNano(), random, ext)
	path := filepath.Join(s.BasePath, name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600) // #nosec G304 -- path built from filepath.Base input
	if err != nil {
		return "", "", fmt.Errorf("open attachment file: %w", err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			_ = closeErr
		}
	}()

	if _, err := io.Copy(f, r); err != nil {
		_ = os.Remove(path)
		return "", "", fmt.Errorf("write attachment: %w", err)
	}
	return name, path, nil
}

func (s *FileStorage) Open(storedName string) (*os.File, error) {
	name := filepath.Base(storedName)
	if name != storedName {
		return nil, fmt.Errorf("invalid attachment name")
	}
	path := filepath.Join(s.BasePath, name)
	f, err := os.Open(path) // #nosec G304 -- storedName restricted to base name
	if err != nil {
		return nil, fmt.Errorf("open attachment: %w", err)
	}
	return f, nil
}
