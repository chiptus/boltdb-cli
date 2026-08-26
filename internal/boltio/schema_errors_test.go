package boltio_test

import (
	"testing"

	"github.com/chiptus/boltdb-cli/internal/boltio"
	bolt "go.etcd.io/bbolt"
)

func TestGetSchemaMissingBucketErrors(t *testing.T) {
	db := newTestDB(t)

	err := db.View(func(tx *bolt.Tx) error {
		_, err := boltio.GetSchema(tx, "does-not-exist", "")
		return err
	})
	if err == nil {
		t.Fatal("expected error for missing bucket")
	}
}

func TestGetSchemaEmptyBucketErrors(t *testing.T) {
	db := newTestDB(t)
	err := db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("empty-bucket"))
		return err
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	err = db.View(func(tx *bolt.Tx) error {
		_, err := boltio.GetSchema(tx, "empty-bucket", "")
		return err
	})
	if err == nil {
		t.Fatal("expected error for empty bucket")
	}
}
