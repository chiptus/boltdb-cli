package cmd_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	bolt "go.etcd.io/bbolt"
)

func newTestPortainerDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "portainer.db")

	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte("version"))
		if err != nil {
			return err
		}
		if err := b.Put([]byte("VERSION"), []byte(`{"SchemaVersion":"2.19.0","MigratorCount":5,"Edition":1,"InstanceID":"abc"}`)); err != nil {
			return err
		}
		return b.Put([]byte("DB_UPDATING"), []byte("true"))
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return path
}

func TestPortainerGetVersionCmd(t *testing.T) {
	path := newTestPortainerDB(t)

	out, err := run(t, "", "portainer", "get-version", "--db", path)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(out, "SchemaVersion: 2.19.0") {
		t.Fatalf("got %q", out)
	}
}

func TestPortainerSetVersionCmdBacksUpBeforeWriting(t *testing.T) {
	path := newTestPortainerDB(t)

	_, err := run(t, "", "portainer", "set-version", "--yes", "--schema-version", "2.20.0", "--db", path)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	out, err := run(t, "", "portainer", "get-version", "--db", path)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(out, "SchemaVersion: 2.20.0") {
		t.Fatalf("schema version not updated: %q", out)
	}
	if !strings.Contains(out, "MigratorCount: 5") {
		t.Fatalf("unrelated field not preserved: %q", out)
	}

	matches, err := filepath.Glob(path + ".bak-*")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected exactly one backup file, got %v", matches)
	}
	if _, err := os.Stat(matches[0]); err != nil {
		t.Fatalf("backup file missing: %v", err)
	}
}

func TestPortainerSetVersionCmdRejectsBadSemver(t *testing.T) {
	path := newTestPortainerDB(t)

	_, err := run(t, "", "portainer", "set-version", "--yes", "--schema-version", "not-a-version", "--db", path)
	if err == nil {
		t.Fatal("expected error for invalid semver")
	}
}

func TestPortainerClearUpdatingFlagCmd(t *testing.T) {
	path := newTestPortainerDB(t)

	_, err := run(t, "", "portainer", "clear-updating-flag", "--yes", "--db", path)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	out, err := run(t, "", "get", "--db", path, "version", "DB_UPDATING")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if strings.TrimSpace(out) != "false" {
		t.Fatalf("got %q", out)
	}
}
