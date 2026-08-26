// Package portainer knows the shape of Portainer's "version" bucket. It is
// a thin, schema-aware wrapper built entirely on top of the generic boltio
// primitives — it does not depend on or vendor any code from
// github.com/portainer/portainer.
package portainer

import (
	"encoding/json"
	"fmt"

	"github.com/chiptus/boltdb-cli/internal/boltio"
	bolt "go.etcd.io/bbolt"
	"golang.org/x/mod/semver"
)

// Bucket is the bbolt bucket Portainer stores its version info in.
const Bucket = "version"

// VersionKey is the key holding the JSON-encoded Version struct.
const VersionKey = "VERSION"

// UpdatingKeyName is the key holding the mid-migration "updating" flag.
const UpdatingKeyName = "DB_UPDATING"

// Version mirrors Portainer's stored version struct
// (package/server-ce/api/database/models/version.go in portainer/portainer-suite).
type Version struct {
	SchemaVersion string
	MigratorCount int
	Edition       int
	InstanceID    string
}

// GetVersion reads and decodes the Version struct stored in tx.
func GetVersion(tx *bolt.Tx) (Version, error) {
	raw, found := boltio.Get(tx, Bucket, VersionKey)
	if !found {
		return Version{}, fmt.Errorf("no %s/%s key found", Bucket, VersionKey)
	}

	var v Version
	if err := json.Unmarshal(raw, &v); err != nil {
		return Version{}, fmt.Errorf("decode version: %w", err)
	}
	return v, nil
}

// SetVersionInput carries the fields the caller wants to change. Fields left
// nil are preserved from the value currently stored on disk.
type SetVersionInput struct {
	SchemaVersion *string
	Edition       *int
	MigratorCount *int
}

// SetVersion reads the current Version struct, mutates only the fields set
// in input, and writes the result back through boltio.Put (so it inherits
// the shared backup/dry-run/confirmation safety net).
func SetVersion(db *bolt.DB, input SetVersionInput, opts boltio.WriteOptions) (boltio.PutResult, error) {
	if input.SchemaVersion != nil && !semver.IsValid("v"+*input.SchemaVersion) {
		return boltio.PutResult{}, fmt.Errorf("%q is not a valid semver version", *input.SchemaVersion)
	}

	var current Version
	err := db.View(func(tx *bolt.Tx) error {
		v, err := GetVersion(tx)
		current = v
		return err
	})
	if err != nil {
		return boltio.PutResult{}, err
	}

	if input.SchemaVersion != nil {
		current.SchemaVersion = *input.SchemaVersion
	}
	if input.Edition != nil {
		current.Edition = *input.Edition
	}
	if input.MigratorCount != nil {
		current.MigratorCount = *input.MigratorCount
	}

	encoded, err := json.Marshal(current)
	if err != nil {
		return boltio.PutResult{}, fmt.Errorf("encode version: %w", err)
	}

	return boltio.Put(db, Bucket, VersionKey, encoded, opts)
}

// ClearUpdatingFlag unsticks a database left with DB_UPDATING=true after a
// crash mid-migration.
func ClearUpdatingFlag(db *bolt.DB, opts boltio.WriteOptions) (boltio.PutResult, error) {
	return boltio.Put(db, Bucket, UpdatingKeyName, []byte("false"), opts)
}
