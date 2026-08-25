// Package boltio provides generic, schema-agnostic primitives for reading
// and writing bucket/key values in a bbolt file, with a shared
// backup-before-write, dry-run, and confirmation-prompt safety net.
package boltio

import (
	"fmt"

	bolt "go.etcd.io/bbolt"
)

// withReadTx opens path read-only, runs fn in a read transaction, and closes
// the file — concentrating the open/close boilerplate every read in this
// package needs in one place.
func withReadTx(path string, fn func(tx *bolt.Tx) error) error {
	db, err := bolt.Open(path, 0600, &bolt.Options{ReadOnly: true})
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = db.Close() }()

	return db.View(fn)
}

// withWriteTx opens path read-write, runs fn in a write transaction, and
// closes the file — the write-side counterpart to withReadTx.
func withWriteTx(path string, fn func(tx *bolt.Tx) error) error {
	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = db.Close() }()

	return db.Update(fn)
}

// listKeysInTx returns the names of every key in bucket, using tx rather
// than opening its own file handle.
func listKeysInTx(tx *bolt.Tx, bucket string) ([]string, error) {
	b := tx.Bucket([]byte(bucket))
	if b == nil {
		return nil, fmt.Errorf("bucket %q not found", bucket)
	}
	var keys []string
	err := b.ForEach(func(key, _ []byte) error {
		keys = append(keys, string(key))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return keys, nil
}

// getInTx returns the value stored at bucket/key, using tx rather than
// opening its own file handle. found is false if the bucket or key does not
// exist.
func getInTx(tx *bolt.Tx, bucket, key string) (value []byte, found bool) {
	b := tx.Bucket([]byte(bucket))
	if b == nil {
		return nil, false
	}
	v := b.Get([]byte(key))
	if v == nil {
		return nil, false
	}
	return append([]byte(nil), v...), true
}
