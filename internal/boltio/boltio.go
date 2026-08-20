// Package boltio provides generic, schema-agnostic primitives for reading
// and writing bucket/key values in a bbolt file, with a shared
// backup-before-write, dry-run, and confirmation-prompt safety net.
package boltio

import (
	"bufio"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"
)

// WriteOptions controls the safety behavior of Put.
type WriteOptions struct {
	// DryRun previews the change without writing or backing up.
	DryRun bool
	// Yes skips the interactive confirmation prompt.
	Yes bool
	// Format renders the old/new preview in this format instead of raw
	// text, so binary values don't print as garbled bytes. One of "",
	// "text", "base64", "hex", or "uint64-be". Empty means "text".
	Format string
	// In is read for the confirmation prompt response. Defaults to os.Stdin.
	In io.Reader
	// Out receives the preview/prompt text. Defaults to os.Stdout.
	Out io.Writer
}

// PutResult reports what Put actually did.
type PutResult struct {
	// Wrote is true if the value was written to the database.
	Wrote bool
	// BackupPath is the path of the pre-write backup, empty if none was made.
	BackupPath string
}

// ListBuckets returns the names of every top-level bucket in the database at path.
func ListBuckets(path string) ([]string, error) {
	db, err := bolt.Open(path, 0600, &bolt.Options{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = db.Close() }()

	var buckets []string
	err = db.View(func(tx *bolt.Tx) error {
		return tx.ForEach(func(name []byte, _ *bolt.Bucket) error {
			buckets = append(buckets, string(name))
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return buckets, nil
}

// ListKeys returns the names of every key in the given bucket.
func ListKeys(path, bucket string) ([]string, error) {
	db, err := bolt.Open(path, 0600, &bolt.Options{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = db.Close() }()

	var keys []string
	err = db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			return fmt.Errorf("bucket %q not found", bucket)
		}
		return b.ForEach(func(key, _ []byte) error {
			keys = append(keys, string(key))
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return keys, nil
}

// Get returns the value stored at bucket/key. found is false if the bucket
// or key does not exist.
func Get(path, bucket, key string) (value []byte, found bool, err error) {
	db, err := bolt.Open(path, 0600, &bolt.Options{ReadOnly: true})
	if err != nil {
		return nil, false, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = db.Close() }()

	err = db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			return nil
		}
		v := b.Get([]byte(key))
		if v == nil {
			return nil
		}
		found = true
		value = append([]byte(nil), v...)
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return value, found, nil
}

// Put writes value to bucket/key, creating the bucket if needed. Every write
// is preceded by a timestamped backup of the file. Callers can preview the
// change without writing via WriteOptions.DryRun, and every non-dry-run
// write is confirmed interactively unless WriteOptions.Yes is set.
func Put(path, bucket, key string, value []byte, opts WriteOptions) (PutResult, error) {
	out := opts.Out
	if out == nil {
		out = os.Stdout
	}
	in := opts.In
	if in == nil {
		in = os.Stdin
	}

	oldValue, found, err := Get(path, bucket, key)
	if err != nil {
		return PutResult{}, err
	}

	render := func(v []byte) string {
		switch opts.Format {
		case "base64":
			return base64.StdEncoding.EncodeToString(v)
		case "hex":
			return hex.EncodeToString(v)
		case "uint64-be":
			if len(v) != 8 {
				return fmt.Sprintf("<uint64-be: value is %d bytes, want 8>", len(v))
			}
			return strconv.FormatUint(binary.BigEndian.Uint64(v), 10)
		default:
			return string(v)
		}
	}

	if _, err := fmt.Fprintf(out, "bucket=%q key=%q\n", bucket, key); err != nil {
		return PutResult{}, err
	}
	if found {
		if _, err := fmt.Fprintf(out, "  old: %s\n", render(oldValue)); err != nil {
			return PutResult{}, err
		}
	} else {
		if _, err := fmt.Fprintf(out, "  old: <absent>\n"); err != nil {
			return PutResult{}, err
		}
	}
	if _, err := fmt.Fprintf(out, "  new: %s\n", render(value)); err != nil {
		return PutResult{}, err
	}

	if opts.DryRun {
		if _, err := fmt.Fprintln(out, "(dry run, no changes written)"); err != nil {
			return PutResult{}, err
		}
		return PutResult{}, nil
	}

	if !opts.Yes {
		if _, err := fmt.Fprint(out, "Write this change? [y/N] "); err != nil {
			return PutResult{}, err
		}
		reader := bufio.NewReader(in)
		line, _ := reader.ReadString('\n')
		if !confirmed(line) {
			if _, err := fmt.Fprintln(out, "Aborted, nothing written."); err != nil {
				return PutResult{}, err
			}
			return PutResult{}, nil
		}
	}

	backupPath, err := backup(path)
	if err != nil {
		return PutResult{}, fmt.Errorf("backup: %w", err)
	}

	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		return PutResult{}, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = db.Close() }()

	err = db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(bucket))
		if err != nil {
			return err
		}
		return b.Put([]byte(key), value)
	})
	if err != nil {
		return PutResult{}, err
	}

	return PutResult{Wrote: true, BackupPath: backupPath}, nil
}

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
