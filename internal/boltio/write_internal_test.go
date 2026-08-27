package boltio

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	bolt "go.etcd.io/bbolt"
)

// TestPutWithOldUsesSuppliedValueNotAFreshRead proves putWithOld trusts the
// knownValue its caller passes in rather than re-reading bucket/key itself
// — the whole point of the split from Put, so Patch doesn't read the key
// twice.
func TestPutWithOldUsesSuppliedValueNotAFreshRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	err = db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte("version"))
		if err != nil {
			return err
		}
		return b.Put([]byte("VERSION"), []byte(`{"SchemaVersion":"on-disk-value"}`))
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	out := &bytes.Buffer{}
	// Deliberately pass an old value that doesn't match what's on disk, to
	// prove the preview reflects the supplied knownValue, not a fresh read.
	res, err := putWithOld(db, "version", "VERSION", []byte(`{"SchemaVersion":"2.20.0"}`), knownValue{
		Bytes: []byte(`{"SchemaVersion":"stale-caller-value"}`),
		Found: true,
	}, WriteOptions{Yes: true, Out: out})
	if err != nil {
		t.Fatalf("putWithOld: %v", err)
	}
	if !res.Wrote {
		t.Fatal("expected write to happen")
	}
	if !strings.Contains(out.String(), "stale-caller-value") {
		t.Fatalf("expected preview to reflect the supplied old value, got %q", out.String())
	}
}
