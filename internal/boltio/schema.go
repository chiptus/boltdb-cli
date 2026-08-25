package boltio

import (
	"encoding/json"
	"fmt"

	bolt "go.etcd.io/bbolt"
)

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
	var result GetSchemaResult
	err := withReadTx(path, func(tx *bolt.Tx) error {
		keys, err := listKeysInTx(tx, bucket)
		if err != nil {
			return err
		}
		if len(keys) == 0 {
			return fmt.Errorf("bucket %q is empty", bucket)
		}

		sampleKey := key
		if sampleKey == "" {
			sampleKey = keys[0]
		}

		value, found := getInTx(tx, bucket, sampleKey)
		if !found {
			return fmt.Errorf("no value at bucket %q key %q", bucket, sampleKey)
		}

		var decoded any
		if err := json.Unmarshal(value, &decoded); err != nil {
			result = GetSchemaResult{Bucket: bucket, SampleKey: sampleKey, NotJSON: true, RawByteLen: len(value)}
			return nil
		}

		shape := inferShape(decoded)
		shape = resolveAmbiguousFields(shape, tx, bucket, sampleKey, keys)

		result = GetSchemaResult{Bucket: bucket, SampleKey: sampleKey, Shape: shape}
		return nil
	})
	if err != nil {
		return GetSchemaResult{}, err
	}
	return result, nil
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
