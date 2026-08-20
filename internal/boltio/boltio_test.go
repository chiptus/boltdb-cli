package boltio_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chiptus/boltdb-cli/internal/boltio"
	bolt "go.etcd.io/bbolt"
)

func newTestDB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte("version"))
		if err != nil {
			return err
		}
		return b.Put([]byte("VERSION"), []byte(`{"SchemaVersion":"2.19.0"}`))
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return path
}

func TestListBuckets(t *testing.T) {
	path := newTestDB(t)

	buckets, err := boltio.ListBuckets(path)
	if err != nil {
		t.Fatalf("ListBuckets: %v", err)
	}
	if len(buckets) != 1 || buckets[0] != "version" {
		t.Fatalf("got %v, want [version]", buckets)
	}
}

func TestListKeys(t *testing.T) {
	path := newTestDB(t)

	keys, err := boltio.ListKeys(path, "version")
	if err != nil {
		t.Fatalf("ListKeys: %v", err)
	}
	if len(keys) != 1 || keys[0] != "VERSION" {
		t.Fatalf("got %v, want [VERSION]", keys)
	}
}

func TestListKeysMissingBucket(t *testing.T) {
	path := newTestDB(t)

	_, err := boltio.ListKeys(path, "does-not-exist")
	if err == nil {
		t.Fatal("expected error for missing bucket")
	}
}

func TestGet(t *testing.T) {
	path := newTestDB(t)

	val, found, err := boltio.Get(path, "version", "VERSION")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !found {
		t.Fatal("expected found=true")
	}
	if string(val) != `{"SchemaVersion":"2.19.0"}` {
		t.Fatalf("got %q", val)
	}
}

func TestGetMissingKey(t *testing.T) {
	path := newTestDB(t)

	_, found, err := boltio.Get(path, "version", "NOPE")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if found {
		t.Fatal("expected found=false")
	}
}

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

	backupData, err := os.ReadFile(res.BackupPath)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if !strings.Contains(string(backupData), "") {
		t.Fatalf("unexpected backup contents")
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

func TestPutPromptsAndAbortsOnNo(t *testing.T) {
	path := newTestDB(t)
	out := &bytes.Buffer{}
	in := strings.NewReader("n\n")

	res, err := boltio.Put(path, "version", "VERSION", []byte(`{"SchemaVersion":"2.20.0"}`), boltio.WriteOptions{
		In:  in,
		Out: out,
	})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if res.Wrote {
		t.Fatal("expected abort on 'n' to not write")
	}
	if res.BackupPath != "" {
		t.Fatal("expected no backup on abort")
	}

	val, _, _ := boltio.Get(path, "version", "VERSION")
	if string(val) != `{"SchemaVersion":"2.19.0"}` {
		t.Fatalf("aborted write mutated the file: got %q", val)
	}
}

func TestPutPromptsAndWritesOnYes(t *testing.T) {
	path := newTestDB(t)
	out := &bytes.Buffer{}
	in := strings.NewReader("y\n")

	res, err := boltio.Put(path, "version", "VERSION", []byte(`{"SchemaVersion":"2.20.0"}`), boltio.WriteOptions{
		In:  in,
		Out: out,
	})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if !res.Wrote {
		t.Fatal("expected confirmed 'y' to write")
	}

	val, _, _ := boltio.Get(path, "version", "VERSION")
	if string(val) != `{"SchemaVersion":"2.20.0"}` {
		t.Fatalf("got %q", val)
	}
}

func TestPutDryRunPreviewIsBase64WhenRequested(t *testing.T) {
	path := newTestDB(t)
	out := &bytes.Buffer{}

	binary := []byte{0xff, 0x00, 0xfe}
	_, err := boltio.Put(path, "version", "BINARY", binary, boltio.WriteOptions{
		DryRun: true,
		Base64: true,
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
