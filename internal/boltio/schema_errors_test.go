package boltio_test

import (
	"testing"

	"github.com/chiptus/boltdb-cli/internal/boltio"
	bolt "go.etcd.io/bbolt"
)

func TestGetSchemaMissingBucketErrors(t *testing.T) {
	path := newTestDB(t)

	_, err := boltio.GetSchema(path, "does-not-exist", "")
	if err == nil {
		t.Fatal("expected error for missing bucket")
	}
}

func TestGetSchemaEmptyBucketErrors(t *testing.T) {
	path := newTestDB(t)
	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("empty-bucket"))
		return err
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	_, err = boltio.GetSchema(path, "empty-bucket", "")
	if err == nil {
		t.Fatal("expected error for empty bucket")
	}
}
