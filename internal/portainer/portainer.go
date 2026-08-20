// Package portainer knows the shape of Portainer's "version" bucket. It is
// a thin, schema-aware wrapper built entirely on top of the generic boltio
// primitives — it does not depend on or vendor any code from
// github.com/portainer/portainer.
package portainer

import (
	"encoding/json"
	"fmt"

	"github.com/chiptus/boltdb-cli/internal/boltio"
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

// GetVersion reads and decodes the Version struct stored at path.
func GetVersion(path string) (Version, error) {
	raw, found, err := boltio.Get(path, Bucket, VersionKey)
	if err != nil {
		return Version{}, err
	}
	if !found {
		return Version{}, fmt.Errorf("no %s/%s key found in %s", Bucket, VersionKey, path)
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
func SetVersion(path string, input SetVersionInput, opts boltio.WriteOptions) (boltio.PutResult, error) {
	if input.SchemaVersion != nil && !semver.IsValid("v"+*input.SchemaVersion) {
		return boltio.PutResult{}, fmt.Errorf("%q is not a valid semver version", *input.SchemaVersion)
	}

	current, err := GetVersion(path)
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

	return boltio.Put(path, Bucket, VersionKey, encoded, opts)
}

// ClearUpdatingFlag unsticks a database left with DB_UPDATING=true after a
// crash mid-migration.
func ClearUpdatingFlag(path string, opts boltio.WriteOptions) (boltio.PutResult, error) {
	return boltio.Put(path, Bucket, UpdatingKeyName, []byte("false"), opts)
}
