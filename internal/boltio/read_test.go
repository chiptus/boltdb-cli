package boltio_test

import (
	"testing"

	"github.com/chiptus/boltdb-cli/internal/boltio"
	"github.com/chiptus/boltdb-cli/internal/valuefmt"
	bolt "go.etcd.io/bbolt"
)

func TestListBuckets(t *testing.T) {
	db := newTestDB(t)

	var buckets []string
	err := db.View(func(tx *bolt.Tx) error {
		buckets = boltio.ListBuckets(tx)
		return nil
	})
	if err != nil {
		t.Fatalf("ListBuckets: %v", err)
	}
	if len(buckets) != 1 || buckets[0] != "version" {
		t.Fatalf("got %v, want [version]", buckets)
	}
}

func TestListKeys(t *testing.T) {
	db := newTestDB(t)

	var keys []string
	err := db.View(func(tx *bolt.Tx) error {
		var err error
		keys, err = boltio.ListKeys(tx, "version")
		return err
	})
	if err != nil {
		t.Fatalf("ListKeys: %v", err)
	}
	if len(keys) != 1 || keys[0] != "VERSION" {
		t.Fatalf("got %v, want [VERSION]", keys)
	}
}

func TestListKeysMissingBucket(t *testing.T) {
	db := newTestDB(t)

	err := db.View(func(tx *bolt.Tx) error {
		_, err := boltio.ListKeys(tx, "does-not-exist")
		return err
	})
	if err == nil {
		t.Fatal("expected error for missing bucket")
	}
}

func TestGuessKeyFormatUint64BE(t *testing.T) {
	db := seedBucket(t, "teams", []byte{0, 0, 0, 0, 0, 0, 0, 1}, []byte("team-1"))

	var got valuefmt.Format
	err := db.View(func(tx *bolt.Tx) error {
		var err error
		got, err = boltio.GuessKeyFormat(tx, "teams")
		return err
	})
	if err != nil {
		t.Fatalf("GuessKeyFormat: %v", err)
	}
	if got != "uint64-be" {
		t.Fatalf("got %q, want uint64-be", got)
	}
}

func TestGuessKeyFormatText(t *testing.T) {
	db := newTestDB(t)

	var got valuefmt.Format
	err := db.View(func(tx *bolt.Tx) error {
		var err error
		got, err = boltio.GuessKeyFormat(tx, "version")
		return err
	})
	if err != nil {
		t.Fatalf("GuessKeyFormat: %v", err)
	}
	if got != "text" {
		t.Fatalf("got %q, want text", got)
	}
}

func TestGuessKeyFormatEightByteTextKeyStillGuessesText(t *testing.T) {
	db := seedBucket(t, "codes", []byte("ABCDEFGH"), []byte("value"))

	var got valuefmt.Format
	err := db.View(func(tx *bolt.Tx) error {
		var err error
		got, err = boltio.GuessKeyFormat(tx, "codes")
		return err
	})
	if err != nil {
		t.Fatalf("GuessKeyFormat: %v", err)
	}
	if got != "text" {
		t.Fatalf("got %q, want text", got)
	}
}

func TestGuessKeyFormatEmptyBucket(t *testing.T) {
	db := newTestDB(t)
	err := db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("empty"))
		return err
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	var got valuefmt.Format
	err = db.View(func(tx *bolt.Tx) error {
		var err error
		got, err = boltio.GuessKeyFormat(tx, "empty")
		return err
	})
	if err != nil {
		t.Fatalf("GuessKeyFormat: %v", err)
	}
	if got != "" {
		t.Fatalf("got %q, want empty guess for an empty bucket", got)
	}
}

func TestGet(t *testing.T) {
	db := newTestDB(t)

	var val []byte
	var found bool
	err := db.View(func(tx *bolt.Tx) error {
		val, found = boltio.Get(tx, "version", "VERSION")
		return nil
	})
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
	db := newTestDB(t)

	var found bool
	err := db.View(func(tx *bolt.Tx) error {
		_, found = boltio.Get(tx, "version", "NOPE")
		return nil
	})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if found {
		t.Fatal("expected found=false")
	}
}
