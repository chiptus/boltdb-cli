package boltio

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

func confirmed(line string) bool {
	line = strings.ToLower(strings.TrimSpace(line))
	return line == "y" || line == "yes"
}

// backup copies path to a timestamped sibling file and returns its path.
func backup(path string) (string, error) {
	backupPath := fmt.Sprintf("%s.bak-%s", path, time.Now().Format("20060102T150405"))

	src, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = src.Close() }()

	dst, err := os.OpenFile(backupPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return "", err
	}
	defer func() { _ = dst.Close() }()

	if _, err := io.Copy(dst, src); err != nil {
		return "", err
	}
	return backupPath, nil
}
