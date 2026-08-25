package boltio

import (
	bolt "go.etcd.io/bbolt"
)

// ListBuckets returns the names of every top-level bucket in the database at path.
func ListBuckets(path string) ([]string, error) {
	var buckets []string
	err := withReadTx(path, func(tx *bolt.Tx) error {
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
	var keys []string
	err := withReadTx(path, func(tx *bolt.Tx) error {
		var err error
		keys, err = listKeysInTx(tx, bucket)
		return err
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
	err = withReadTx(path, func(tx *bolt.Tx) error {
		value, found = getInTx(tx, bucket, key)
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return value, found, nil
}
