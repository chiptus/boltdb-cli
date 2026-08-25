package boltio_test

import (
	"testing"

	"github.com/chiptus/boltdb-cli/internal/boltio"
	bolt "go.etcd.io/bbolt"
)

func TestListBuckets(t *testing.T) {
	path := newTestDB(t)

	buckets, err := boltio.ListBuckets(path)
	if err != nil {
		t.Fatalf("ListBuckets: %v", err)
	}
	if len(buckets) != 1 || buckets[0] != "version" {
		t.Fatalf("got %v, want [version]", buckets)
	}
}

func TestListKeys(t *testing.T) {
	path := newTestDB(t)

	keys, err := boltio.ListKeys(path, "version")
	if err != nil {
		t.Fatalf("ListKeys: %v", err)
	}
	if len(keys) != 1 || keys[0] != "VERSION" {
		t.Fatalf("got %v, want [VERSION]", keys)
	}
}

func TestListKeysMissingBucket(t *testing.T) {
	path := newTestDB(t)

	_, err := boltio.ListKeys(path, "does-not-exist")
	if err == nil {
		t.Fatal("expected error for missing bucket")
	}
}

func TestGuessKeyFormatUint64BE(t *testing.T) {
	path := seedBucket(t, "teams", []byte{0, 0, 0, 0, 0, 0, 0, 1}, []byte("team-1"))

	got, err := boltio.GuessKeyFormat(path, "teams")
	if err != nil {
		t.Fatalf("GuessKeyFormat: %v", err)
	}
	if got != "uint64-be" {
		t.Fatalf("got %q, want uint64-be", got)
	}
}

func TestGuessKeyFormatText(t *testing.T) {
	path := newTestDB(t)

	got, err := boltio.GuessKeyFormat(path, "version")
	if err != nil {
		t.Fatalf("GuessKeyFormat: %v", err)
	}
	if got != "text" {
		t.Fatalf("got %q, want text", got)
	}
}

func TestGuessKeyFormatEightByteTextKeyStillGuessesText(t *testing.T) {
	path := seedBucket(t, "codes", []byte("ABCDEFGH"), []byte("value"))

	got, err := boltio.GuessKeyFormat(path, "codes")
	if err != nil {
		t.Fatalf("GuessKeyFormat: %v", err)
	}
	if got != "text" {
		t.Fatalf("got %q, want text", got)
	}
}

func TestGuessKeyFormatEmptyBucket(t *testing.T) {
	path := newTestDB(t)
	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("empty"))
		return err
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	got, err := boltio.GuessKeyFormat(path, "empty")
	if err != nil {
		t.Fatalf("GuessKeyFormat: %v", err)
	}
	if got != "" {
		t.Fatalf("got %q, want empty guess for an empty bucket", got)
	}
}

func TestGet(t *testing.T) {
	path := newTestDB(t)

	val, found, err := boltio.Get(path, "version", "VERSION")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !found {
		t.Fatal("expected found=true")
	}
	if string(val) != `{"SchemaVersion":"2.19.0"}` {
		t.Fatalf("got %q", val)
	}
}

func TestGetMissingKey(t *testing.T) {
	path := newTestDB(t)

	_, found, err := boltio.Get(path, "version", "NOPE")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if found {
		t.Fatal("expected found=false")
	}
}
