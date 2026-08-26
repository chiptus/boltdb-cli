package boltio

import (
	"fmt"

	"github.com/chiptus/boltdb-cli/internal/valuefmt"
	bolt "go.etcd.io/bbolt"
)

// ListBuckets returns the names of every top-level bucket in the database.
func ListBuckets(tx *bolt.Tx) []string {
	var buckets []string
	_ = tx.ForEach(func(name []byte, _ *bolt.Bucket) error {
		buckets = append(buckets, string(name))
		return nil
	})
	return buckets
}

// ListKeys returns the names of every key in the given bucket.
func ListKeys(tx *bolt.Tx, bucket string) ([]string, error) {
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

// keyFormatSampleSize bounds how many of a bucket's keys GuessKeyFormat
// inspects before settling on a guess.
const keyFormatSampleSize = 20

// GuessKeyFormat samples up to keyFormatSampleSize keys from bucket and
// guesses the format they were most likely written in: "uint64-be" if every
// sampled key is exactly 8 raw bytes that don't look like printable text
// (the shape of a bolt.Bucket.NextSequence() key), otherwise "text". Returns
// "" if the bucket has no keys to sample, leaving the choice of default to
// the caller.
func GuessKeyFormat(tx *bolt.Tx, bucket string) (valuefmt.Format, error) {
	keys, err := ListKeys(tx, bucket)
	if err != nil {
		return "", err
	}
	if len(keys) == 0 {
		return "", nil
	}

	limit := min(len(keys), keyFormatSampleSize)
	for _, k := range keys[:limit] {
		if len(k) != 8 || looksLikeText([]byte(k)) {
			return valuefmt.Text, nil
		}
	}
	return valuefmt.Uint64BE, nil
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
func Get(tx *bolt.Tx, bucket, key string) (value []byte, found bool) {
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
