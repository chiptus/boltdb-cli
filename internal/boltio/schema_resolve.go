package boltio

import (
	"encoding/json"

	bolt "go.etcd.io/bbolt"
)

// fallbackScanLimit bounds how many additional keys GetSchema will check,
// in total, when trying to resolve ambiguous fields to a concrete type.
const fallbackScanLimit = 20

// resolveAmbiguousFields finds every ambiguous field in shape (recursively,
// through nested objects) and resolves as many as it can in a single
// shared scan of up to fallbackScanLimit other keys in the bucket: each
// candidate key's value is fetched and decoded once, then checked against
// every field still unresolved, stopping early once none remain.
func resolveAmbiguousFields(shape Shape, tx *bolt.Tx, bucket, sampleKey string, keys []string) Shape {
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

		value, found := Get(tx, bucket, k)
		if !found {
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
