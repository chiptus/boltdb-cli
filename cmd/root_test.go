package cmd_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chiptus/boltdb-cli/cmd"
	bolt "go.etcd.io/bbolt"
)

func newTestDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")

	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte("greeting"))
		if err != nil {
			return err
		}
		return b.Put([]byte("hello"), []byte("world"))
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return path
}

func run(t *testing.T, in string, args ...string) (stdout string, err error) {
	t.Helper()
	root := cmd.NewRootCmd()
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(out)
	root.SetIn(strings.NewReader(in))
	root.SetArgs(args)
	err = root.Execute()
	return out.String(), err
}

func TestListBucketsCmd(t *testing.T) {
	path := newTestDB(t)

	out, err := run(t, "", "list-buckets", path)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if strings.TrimSpace(out) != "greeting" {
		t.Fatalf("got %q", out)
	}
}

func TestListKeysCmd(t *testing.T) {
	path := newTestDB(t)

	out, err := run(t, "", "list-keys", path, "greeting")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if strings.TrimSpace(out) != "hello" {
		t.Fatalf("got %q", out)
	}
}

func TestListKeysCmdBase64ForBinaryKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte("teams"))
		if err != nil {
			return err
		}
		// Mimics Portainer's NextSequence()-derived binary keys.
		return b.Put([]byte{0, 0, 0, 0, 0, 0, 0, 1}, []byte("team-1"))
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	out, err := run(t, "", "list-keys", "--format", "base64", path, "teams")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if strings.TrimSpace(out) != "AAAAAAAAAAE=" {
		t.Fatalf("got %q", out)
	}
}

func TestListKeysCmdUint64BEForSequenceKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte("teams"))
		if err != nil {
			return err
		}
		return b.Put([]byte{0, 0, 0, 0, 0, 0, 0, 1}, []byte("team-1"))
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	out, err := run(t, "", "list-keys", "--format", "uint64-be", path, "teams")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if strings.TrimSpace(out) != "1" {
		t.Fatalf("got %q", out)
	}
}

func TestGetCmdUint64BEKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte("users"))
		if err != nil {
			return err
		}
		return b.Put([]byte{0, 0, 0, 0, 0, 0, 0, 1}, []byte("admin"))
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	out, err := run(t, "", "get", "--key-format", "uint64-be", path, "users", "1")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if strings.TrimSpace(out) != "admin" {
		t.Fatalf("got %q", out)
	}
}

func TestGetCmd(t *testing.T) {
	path := newTestDB(t)

	out, err := run(t, "", "get", path, "greeting", "hello")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if strings.TrimSpace(out) != "world" {
		t.Fatalf("got %q", out)
	}
}

func TestGetCmdBase64(t *testing.T) {
	path := newTestDB(t)

	out, err := run(t, "", "get", "--format", "base64", path, "greeting", "hello")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if strings.TrimSpace(out) != "d29ybGQ=" {
		t.Fatalf("got %q", out)
	}
}

func TestPutCmdDryRunDoesNotWrite(t *testing.T) {
	path := newTestDB(t)

	_, err := run(t, "", "put", "--dry-run", path, "greeting", "hello", "changed")
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	out, err := run(t, "", "get", path, "greeting", "hello")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if strings.TrimSpace(out) != "world" {
		t.Fatalf("dry-run should not have written, got %q", out)
	}
}

func TestPutCmdWithYesWrites(t *testing.T) {
	path := newTestDB(t)

	_, err := run(t, "", "put", "--yes", path, "greeting", "hello", "changed")
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	out, err := run(t, "", "get", path, "greeting", "hello")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if strings.TrimSpace(out) != "changed" {
		t.Fatalf("got %q", out)
	}
}

func TestPutCmdBase64Roundtrip(t *testing.T) {
	path := newTestDB(t)

	_, err := run(t, "", "put", "--yes", "--format", "base64", path, "greeting", "hello", "aGVsbG8=")
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	out, err := run(t, "", "get", path, "greeting", "hello")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if strings.TrimSpace(out) != "hello" {
		t.Fatalf("got %q", out)
	}
}

func TestPutCmdPromptAbortsWithoutYes(t *testing.T) {
	path := newTestDB(t)

	_, err := run(t, "n\n", "put", path, "greeting", "hello", "changed")
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	out, err := run(t, "", "get", path, "greeting", "hello")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if strings.TrimSpace(out) != "world" {
		t.Fatalf("expected abort to leave value unchanged, got %q", out)
	}
}
