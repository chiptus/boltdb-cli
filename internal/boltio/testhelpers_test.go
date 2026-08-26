package boltio_test

import (
	"path/filepath"
	"testing"

	bolt "go.etcd.io/bbolt"
)

// newTestDB opens a temp bbolt file seeded with a "version"/"VERSION" key,
// and registers t.Cleanup to close it.
func newTestDB(t *testing.T) *bolt.DB {
	t.Helper()
	return seedBucket(t, "version", []byte("VERSION"), []byte(`{"SchemaVersion":"2.19.0"}`))
}

// seedBucket opens a temp bbolt file with a single bucket/key/value seeded,
// and registers t.Cleanup to close it.
func seedBucket(t *testing.T, bucket string, key, value []byte) *bolt.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")

	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	err = db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(bucket))
		if err != nil {
			return err
		}
		return b.Put(key, value)
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	return db
}
