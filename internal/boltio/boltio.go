// Package boltio provides generic, schema-agnostic primitives for reading
// and writing bucket/key values in a bbolt file, with a shared
// backup-before-write, dry-run, and confirmation-prompt safety net.
package boltio

import (
	"bufio"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	jsonpatch "github.com/evanphx/json-patch/v5"
	bolt "go.etcd.io/bbolt"
)

// WriteOptions controls the safety behavior of Put.
type WriteOptions struct {
	// DryRun previews the change without writing or backing up.
	DryRun bool
	// Yes skips the interactive confirmation prompt.
	Yes bool
	// Format renders the old/new preview in this format instead of raw
	// text, so binary values don't print as garbled bytes. One of "",
	// "text", "base64", "hex", or "uint64-be". Empty means "text".
	Format string
	// In is read for the confirmation prompt response. Defaults to os.Stdin.
	In io.Reader
	// Out receives the preview/prompt text. Defaults to os.Stdout.
	Out io.Writer
}

// PutResult reports what Put actually did.
type PutResult struct {
	// Wrote is true if the value was written to the database.
	Wrote bool
	// BackupPath is the path of the pre-write backup, empty if none was made.
	BackupPath string
}

// ListBuckets returns the names of every top-level bucket in the database at path.
func ListBuckets(path string) ([]string, error) {
	db, err := bolt.Open(path, 0600, &bolt.Options{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = db.Close() }()

	var buckets []string
	err = db.View(func(tx *bolt.Tx) error {
		return tx.ForEach(func(name []byte, _ *bolt.Bucket) error {
			buckets = append(buckets, string(name))
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return buckets, nil
}

// ListKeys returns the names of every key in the given bucket.
func ListKeys(path, bucket string) ([]string, error) {
	db, err := bolt.Open(path, 0600, &bolt.Options{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = db.Close() }()

	var keys []string
	err = db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			return fmt.Errorf("bucket %q not found", bucket)
		}
		return b.ForEach(func(key, _ []byte) error {
			keys = append(keys, string(key))
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return keys, nil
}

// keyFormatSampleSize bounds how many of a bucket's keys GuessKeyFormat
// inspects before settling on a guess.
const keyFormatSampleSize = 20

// GuessKeyFormat samples up to keyFormatSampleSize keys from bucket and
// guesses the format they were most likely written in: "uint64-be" if every
// sampled key is exactly 8 raw bytes that don't look like printable text
// (the shape of a bolt.Bucket.NextSequence() key), otherwise "text". Returns
// "" if the bucket has no keys to sample, leaving the choice of default to
// the caller.
func GuessKeyFormat(path, bucket string) (string, error) {
	keys, err := ListKeys(path, bucket)
	if err != nil {
		return "", err
	}
	if len(keys) == 0 {
		return "", nil
	}

	limit := min(len(keys), keyFormatSampleSize)
	for _, k := range keys[:limit] {
		if len(k) != 8 || looksLikeText([]byte(k)) {
			return "text", nil
		}
	}
	return "uint64-be", nil
}

// looksLikeText reports whether b is entirely printable ASCII, the shape of
// a human-authored text key as opposed to a packed binary encoding.
func looksLikeText(b []byte) bool {
	for _, c := range b {
		if c < 0x20 || c > 0x7e {
			return false
		}
	}
	return true
}

// Get returns the value stored at bucket/key. found is false if the bucket
// or key does not exist.
func Get(path, bucket, key string) (value []byte, found bool, err error) {
	db, err := bolt.Open(path, 0600, &bolt.Options{ReadOnly: true})
	if err != nil {
		return nil, false, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = db.Close() }()

	err = db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			return nil
		}
		v := b.Get([]byte(key))
		if v == nil {
			return nil
		}
		found = true
		value = append([]byte(nil), v...)
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return value, found, nil
}

// Put writes value to bucket/key, creating the bucket if needed. Every write
// is preceded by a timestamped backup of the file. Callers can preview the
// change without writing via WriteOptions.DryRun, and every non-dry-run
// write is confirmed interactively unless WriteOptions.Yes is set.
func Put(path, bucket, key string, value []byte, opts WriteOptions) (PutResult, error) {
	out := opts.Out
	if out == nil {
		out = os.Stdout
	}
	in := opts.In
	if in == nil {
		in = os.Stdin
	}

	oldValue, found, err := Get(path, bucket, key)
	if err != nil {
		return PutResult{}, err
	}

	render := func(v []byte) string {
		switch opts.Format {
		case "base64":
			return base64.StdEncoding.EncodeToString(v)
		case "hex":
			return hex.EncodeToString(v)
		case "uint64-be":
			if len(v) != 8 {
				return fmt.Sprintf("<uint64-be: value is %d bytes, want 8>", len(v))
			}
			return strconv.FormatUint(binary.BigEndian.Uint64(v), 10)
		default:
			return string(v)
		}
	}

	if _, err := fmt.Fprintf(out, "bucket=%q key=%q\n", bucket, key); err != nil {
		return PutResult{}, err
	}
	if found {
		if _, err := fmt.Fprintf(out, "  old: %s\n", render(oldValue)); err != nil {
			return PutResult{}, err
		}
	} else {
		if _, err := fmt.Fprintf(out, "  old: <absent>\n"); err != nil {
			return PutResult{}, err
		}
	}
	if _, err := fmt.Fprintf(out, "  new: %s\n", render(value)); err != nil {
		return PutResult{}, err
	}

	if opts.DryRun {
		if _, err := fmt.Fprintln(out, "(dry run, no changes written)"); err != nil {
			return PutResult{}, err
		}
		return PutResult{}, nil
	}

	if !opts.Yes {
		if _, err := fmt.Fprint(out, "Write this change? [y/N] "); err != nil {
			return PutResult{}, err
		}
		reader := bufio.NewReader(in)
		line, _ := reader.ReadString('\n')
		if !confirmed(line) {
			if _, err := fmt.Fprintln(out, "Aborted, nothing written."); err != nil {
				return PutResult{}, err
			}
			return PutResult{}, nil
		}
	}

	backupPath, err := backup(path)
	if err != nil {
		return PutResult{}, fmt.Errorf("backup: %w", err)
	}

	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		return PutResult{}, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = db.Close() }()

	err = db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(bucket))
		if err != nil {
			return err
		}
		return b.Put([]byte(key), value)
	})
	if err != nil {
		return PutResult{}, err
	}

	return PutResult{Wrote: true, BackupPath: backupPath}, nil
}

// Patch reads the JSON object stored at bucket/key, applies fragment to it
// as an RFC 7396 JSON Merge Patch (a null value in fragment deletes the
// corresponding key), and writes the merged result back through Put, so it
// inherits the same backup/dry-run/confirmation safety net.
func Patch(path, bucket, key string, fragment []byte, opts WriteOptions) (PutResult, error) {
	current, found, err := Get(path, bucket, key)
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

	return Put(path, bucket, key, merged, opts)
}

// fallbackScanLimit bounds how many additional keys GetSchema will check,
// in total, when trying to resolve ambiguous fields to a concrete type.
const fallbackScanLimit = 20

// Shape describes the inferred type of a single JSON value: a scalar kind
// ("string", "number", "bool", "null"), "object" (with Fields populated),
// or "array" (with Elem describing the first element's shape, nil for an
// empty array).
type Shape struct {
	Kind   string
	Fields map[string]Shape
	Elem   *Shape

	// ResolvedKey is set when this shape started out ambiguous (null, or
	// an empty array) in the primary sample and was resolved by scanning
	// another key in the bucket for a concrete value at the same field.
	ResolvedKey string
}

// GetSchemaResult is the result of inferring a bucket's shape from a sample value.
type GetSchemaResult struct {
	Bucket     string
	SampleKey  string
	NotJSON    bool
	RawByteLen int
	Shape      Shape
}

// GetSchema infers the field shape of a sample value in bucket, for use as
// an agent- or human-facing approximation of the bucket's schema (bbolt
// buckets have no real schema, so this is always inferred from a sample,
// never a guarantee). If key is empty, the bucket's first key (per
// ListKeys) is sampled; otherwise the given key is sampled directly.
//
// Fields whose sampled value is ambiguous (null, or an empty array) are
// resolved, when possible, by scanning up to fallbackScanLimit other keys
// in the bucket for a concrete value at that same field, stopping at the
// first one found.
func GetSchema(path, bucket, key string) (GetSchemaResult, error) {
	keys, err := ListKeys(path, bucket)
	if err != nil {
		return GetSchemaResult{}, err
	}
	if len(keys) == 0 {
		return GetSchemaResult{}, fmt.Errorf("bucket %q is empty", bucket)
	}

	sampleKey := key
	if sampleKey == "" {
		sampleKey = keys[0]
	}

	value, found, err := Get(path, bucket, sampleKey)
	if err != nil {
		return GetSchemaResult{}, err
	}
	if !found {
		return GetSchemaResult{}, fmt.Errorf("no value at bucket %q key %q", bucket, sampleKey)
	}

	var decoded any
	if err := json.Unmarshal(value, &decoded); err != nil {
		return GetSchemaResult{Bucket: bucket, SampleKey: sampleKey, NotJSON: true, RawByteLen: len(value)}, nil
	}

	shape := inferShape(decoded)
	shape = resolveAmbiguousFields(shape, path, bucket, sampleKey, keys)

	return GetSchemaResult{Bucket: bucket, SampleKey: sampleKey, Shape: shape}, nil
}

// inferShape infers the Shape of a JSON value already decoded via
// encoding/json into Go's standard any representation: nil, bool, float64,
// string, []any, or map[string]any — no other type is possible here.
func inferShape(v any) Shape {
	switch t := v.(type) {
	case nil:
		return Shape{Kind: "null"}
	case bool:
		return Shape{Kind: "bool"}
	case float64:
		return Shape{Kind: "number"}
	case string:
		return Shape{Kind: "string"}
	case []any:
		if len(t) == 0 {
			return Shape{Kind: "array"}
		}
		elem := inferShape(t[0])
		return Shape{Kind: "array", Elem: &elem}
	case map[string]any:
		fields := make(map[string]Shape, len(t))
		for k, fv := range t {
			fields[k] = inferShape(fv)
		}
		return Shape{Kind: "object", Fields: fields}
	}
	panic(fmt.Sprintf("inferShape: unexpected decoded JSON type %T", v))
}

// isAmbiguousShape reports whether s carries no useful type information on
// its own: a null value, or an empty array.
func isAmbiguousShape(s Shape) bool {
	return s.Kind == "null" || (s.Kind == "array" && s.Elem == nil)
}

// resolveAmbiguousFields finds every ambiguous field in shape (recursively,
// through nested objects) and resolves as many as it can in a single
// shared scan of up to fallbackScanLimit other keys in the bucket: each
// candidate key's value is fetched and decoded once, then checked against
// every field still unresolved, stopping early once none remain.
func resolveAmbiguousFields(shape Shape, dbPath, bucket, sampleKey string, keys []string) Shape {
	remaining := collectAmbiguousPaths(shape, nil)
	if len(remaining) == 0 {
		return shape
	}

	checked := 0
	for _, k := range keys {
		if len(remaining) == 0 || checked >= fallbackScanLimit {
			break
		}
		if k == sampleKey {
			continue
		}
		checked++

		value, found, err := Get(dbPath, bucket, k)
		if err != nil || !found {
			continue
		}
		var decoded any
		if err := json.Unmarshal(value, &decoded); err != nil {
			continue
		}

		var stillRemaining [][]string
		for _, fieldPath := range remaining {
			fieldValue, ok := navigate(decoded, fieldPath)
			if !ok {
				stillRemaining = append(stillRemaining, fieldPath)
				continue
			}
			candidate := inferShape(fieldValue)
			if isAmbiguousShape(candidate) {
				stillRemaining = append(stillRemaining, fieldPath)
				continue
			}
			candidate.ResolvedKey = k
			shape = shapeWithFieldAt(shape, fieldPath, candidate)
		}
		remaining = stillRemaining
	}

	return shape
}

// collectAmbiguousPaths returns the field path (sequence of nested object
// key names) of every ambiguous field found by walking shape's object
// fields recursively.
func collectAmbiguousPaths(shape Shape, path []string) [][]string {
	if shape.Kind != "object" {
		return nil
	}
	var paths [][]string
	for name, field := range shape.Fields {
		fieldPath := append(append([]string(nil), path...), name)
		if isAmbiguousShape(field) {
			paths = append(paths, fieldPath)
		} else {
			paths = append(paths, collectAmbiguousPaths(field, fieldPath)...)
		}
	}
	return paths
}

// shapeWithFieldAt returns a copy of shape with the field at path replaced
// by resolved.
func shapeWithFieldAt(shape Shape, path []string, resolved Shape) Shape {
	if len(path) == 0 {
		return resolved
	}
	fields := make(map[string]Shape, len(shape.Fields))
	for name, field := range shape.Fields {
		if name == path[0] {
			field = shapeWithFieldAt(field, path[1:], resolved)
		}
		fields[name] = field
	}
	shape.Fields = fields
	return shape
}

// navigate walks v through a sequence of object field names, returning the
// value found at path, or ok=false if v isn't shaped like an object at any
// point along the way.
func navigate(v any, path []string) (any, bool) {
	cur := v
	for _, seg := range path {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = m[seg]
		if !ok {
			return nil, false
		}
	}
	return cur, true
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

func confirmed(line string) bool {
	line = strings.ToLower(strings.TrimSpace(line))
	return line == "y" || line == "yes"
}

// backup copies path to a timestamped sibling file and returns its path.
func backup(path string) (string, error) {
	backupPath := fmt.Sprintf("%s.bak-%s", path, time.Now().Format("20060102T150405"))

	src, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = src.Close() }()

	dst, err := os.OpenFile(backupPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return "", err
	}
	defer func() { _ = dst.Close() }()

	if _, err := io.Copy(dst, src); err != nil {
		return "", err
	}
	return backupPath, nil
}
