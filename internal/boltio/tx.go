// Package boltio provides generic, schema-agnostic primitives for reading
// and writing bucket/key values in a bbolt file, with a shared
// backup-before-write, dry-run, and confirmation-prompt safety net.
package boltio

import (
	"fmt"

	bolt "go.etcd.io/bbolt"
)

// OpenRead opens path read-only. Callers drive db.View themselves, running
// as many of this package's tx-scoped functions as they need in one
// transaction, then close db when done.
func OpenRead(path string) (*bolt.DB, error) {
	db, err := bolt.Open(path, 0600, &bolt.Options{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	return db, nil
}

// OpenWrite opens path read-write. Callers drive db.View/db.Update
// themselves — directly, or via this package's Put/Patch — then close db
// when done.
func OpenWrite(path string) (*bolt.DB, error) {
	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	return db, nil
}
