package boltio_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/chiptus/boltdb-cli/internal/boltio"
)

func TestPutDryRunDoesNotWriteOrBackup(t *testing.T) {
	path := newTestDB(t)
	out := &bytes.Buffer{}

	res, err := boltio.Put(path, "version", "VERSION", []byte(`{"SchemaVersion":"2.20.0"}`), boltio.WriteOptions{
		DryRun: true,
		Out:    out,
	})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if res.Wrote {
		t.Fatal("expected dry-run to not write")
	}
	if res.BackupPath != "" {
		t.Fatal("expected dry-run to not create a backup")
	}

	val, _, err := boltio.Get(path, "version", "VERSION")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(val) != `{"SchemaVersion":"2.19.0"}` {
		t.Fatalf("dry-run mutated the file: got %q", val)
	}

	if !strings.Contains(out.String(), "2.19.0") || !strings.Contains(out.String(), "2.20.0") {
		t.Fatalf("expected preview to mention old and new values, got %q", out.String())
	}
}

func TestPutWithYesWritesAndBackups(t *testing.T) {
	path := newTestDB(t)
	out := &bytes.Buffer{}

	res, err := boltio.Put(path, "version", "VERSION", []byte(`{"SchemaVersion":"2.20.0"}`), boltio.WriteOptions{
		Yes: true,
		Out: out,
	})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if !res.Wrote {
		t.Fatal("expected write to happen")
	}
	if res.BackupPath == "" {
		t.Fatal("expected a backup path")
	}

	val, _, err := boltio.Get(path, "version", "VERSION")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(val) != `{"SchemaVersion":"2.20.0"}` {
		t.Fatalf("got %q", val)
	}

	backupVal, _, err := boltio.Get(res.BackupPath, "version", "VERSION")
	if err != nil {
		t.Fatalf("Get on backup: %v", err)
	}
	if string(backupVal) != `{"SchemaVersion":"2.19.0"}` {
		t.Fatalf("backup should hold the pre-write value, got %q", backupVal)
	}
}

func TestPutDryRunPreviewIsBase64WhenRequested(t *testing.T) {
	path := newTestDB(t)
	out := &bytes.Buffer{}

	binary := []byte{0xff, 0x00, 0xfe}
	_, err := boltio.Put(path, "version", "BINARY", binary, boltio.WriteOptions{
		DryRun: true,
		Format: "base64",
		Out:    out,
	})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	if !strings.Contains(out.String(), "/wD+") { // base64 of 0xff 0x00 0xfe
		t.Fatalf("expected base64-encoded preview, got %q", out.String())
	}
}

func TestPutCreatesBucketIfMissing(t *testing.T) {
	path := newTestDB(t)

	res, err := boltio.Put(path, "new-bucket", "key1", []byte("hello"), boltio.WriteOptions{Yes: true, Out: &bytes.Buffer{}})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if !res.Wrote {
		t.Fatal("expected write")
	}

	val, found, err := boltio.Get(path, "new-bucket", "key1")
	if err != nil || !found {
		t.Fatalf("Get: found=%v err=%v", found, err)
	}
	if string(val) != "hello" {
		t.Fatalf("got %q", val)
	}
}
