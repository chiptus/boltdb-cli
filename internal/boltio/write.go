package boltio

import (
	"bufio"
	"fmt"
	"io"
	"os"

	bolt "go.etcd.io/bbolt"
)

// WriteOptions controls the safety behavior of Put.
type WriteOptions struct {
	// DryRun previews the change without writing or backing up.
	DryRun bool
	// Yes skips the interactive confirmation prompt.
	Yes bool
	// Render turns a raw value into the string shown in the old/new
	// preview, so binary values don't print as garbled bytes. A nil Render
	// treats the value as plain text. An error from Render aborts the
	// write before the confirmation prompt is shown.
	Render func([]byte) (string, error)
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

	render := opts.Render
	if render == nil {
		render = func(v []byte) (string, error) { return string(v), nil }
	}

	if _, err := fmt.Fprintf(out, "bucket=%q key=%q\n", bucket, key); err != nil {
		return PutResult{}, err
	}
	if found {
		oldRendered, err := render(oldValue)
		if err != nil {
			return PutResult{}, fmt.Errorf("render old value: %w", err)
		}
		if _, err := fmt.Fprintf(out, "  old: %s\n", oldRendered); err != nil {
			return PutResult{}, err
		}
	} else {
		if _, err := fmt.Fprintf(out, "  old: <absent>\n"); err != nil {
			return PutResult{}, err
		}
	}
	newRendered, err := render(value)
	if err != nil {
		return PutResult{}, fmt.Errorf("render new value: %w", err)
	}
	if _, err := fmt.Fprintf(out, "  new: %s\n", newRendered); err != nil {
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

	err = withWriteTx(path, func(tx *bolt.Tx) error {
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
