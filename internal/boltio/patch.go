package boltio

import (
	"encoding/json"
	"fmt"

	jsonpatch "github.com/evanphx/json-patch/v5"
	bolt "go.etcd.io/bbolt"
)

// Patch reads the JSON object stored at bucket/key, applies fragment to it
// as an RFC 7396 JSON Merge Patch (a null value in fragment deletes the
// corresponding key), and writes the merged result back through Put, so it
// inherits the same backup/dry-run/confirmation safety net.
func Patch(db *bolt.DB, bucket, key string, fragment []byte, opts WriteOptions) (PutResult, error) {
	var current []byte
	var found bool
	err := db.View(func(tx *bolt.Tx) error {
		current, found = Get(tx, bucket, key)
		return nil
	})
	if err != nil {
		return PutResult{}, err
	}
	if !found {
		return PutResult{}, fmt.Errorf("no value at bucket %q key %q", bucket, key)
	}

	if err := requireJSONObject(current, "value"); err != nil {
		return PutResult{}, err
	}
	if err := requireJSONObject(fragment, "patch fragment"); err != nil {
		return PutResult{}, err
	}

	merged, err := jsonpatch.MergePatch(current, fragment)
	if err != nil {
		return PutResult{}, fmt.Errorf("apply patch: %w", err)
	}

	return Put(db, bucket, key, merged, opts)
}

// requireJSONObject returns an error if b is not valid JSON, or is valid
// JSON but not a JSON object. label identifies b in the error message.
func requireJSONObject(b []byte, label string) error {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return fmt.Errorf("%s is not valid JSON: %w", label, err)
	}
	if _, ok := v.(map[string]any); !ok {
		return fmt.Errorf("%s is not a JSON object, got %T", label, v)
	}
	return nil
}
