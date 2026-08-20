package portainer_test

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/chiptus/boltdb-cli/internal/boltio"
	"github.com/chiptus/boltdb-cli/internal/portainer"
	bolt "go.etcd.io/bbolt"
)

func newTestDB(t *testing.T, versionJSON string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")

	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte("version"))
		if err != nil {
			return err
		}
		return b.Put([]byte("VERSION"), []byte(versionJSON))
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return path
}

func TestGetVersion(t *testing.T) {
	path := newTestDB(t, `{"SchemaVersion":"2.19.0","MigratorCount":5,"Edition":1,"InstanceID":"abc"}`)

	v, err := portainer.GetVersion(path)
	if err != nil {
		t.Fatalf("GetVersion: %v", err)
	}
	if v.SchemaVersion != "2.19.0" || v.MigratorCount != 5 || v.Edition != 1 || v.InstanceID != "abc" {
		t.Fatalf("got %+v", v)
	}
}

func TestGetVersionMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.db")
	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	_, err = portainer.GetVersion(path)
	if err == nil {
		t.Fatal("expected error when version key is absent")
	}
}

func TestSetVersionUpdatesOnlyRequestedFields(t *testing.T) {
	path := newTestDB(t, `{"SchemaVersion":"2.19.0","MigratorCount":5,"Edition":1,"InstanceID":"abc"}`)
	schemaVersion := "2.20.0"

	res, err := portainer.SetVersion(path, portainer.SetVersionInput{
		SchemaVersion: &schemaVersion,
	}, boltio.WriteOptions{Yes: true, Out: &bytes.Buffer{}})
	if err != nil {
		t.Fatalf("SetVersion: %v", err)
	}
	if !res.Wrote {
		t.Fatal("expected write")
	}

	v, err := portainer.GetVersion(path)
	if err != nil {
		t.Fatalf("GetVersion: %v", err)
	}
	if v.SchemaVersion != "2.20.0" {
		t.Fatalf("schema version not updated: %+v", v)
	}
	if v.MigratorCount != 5 || v.Edition != 1 || v.InstanceID != "abc" {
		t.Fatalf("unrelated fields should be preserved, got %+v", v)
	}
}

func TestSetVersionRejectsInvalidSemver(t *testing.T) {
	path := newTestDB(t, `{"SchemaVersion":"2.19.0"}`)
	bad := "not-a-version"

	_, err := portainer.SetVersion(path, portainer.SetVersionInput{
		SchemaVersion: &bad,
	}, boltio.WriteOptions{Yes: true, Out: &bytes.Buffer{}})
	if err == nil {
		t.Fatal("expected error for invalid semver")
	}
}

func TestSetVersionEditionAndMigratorCount(t *testing.T) {
	path := newTestDB(t, `{"SchemaVersion":"2.19.0","MigratorCount":1,"Edition":1,"InstanceID":"abc"}`)
	edition := 2
	migratorCount := 9

	_, err := portainer.SetVersion(path, portainer.SetVersionInput{
		Edition:       &edition,
		MigratorCount: &migratorCount,
	}, boltio.WriteOptions{Yes: true, Out: &bytes.Buffer{}})
	if err != nil {
		t.Fatalf("SetVersion: %v", err)
	}

	v, err := portainer.GetVersion(path)
	if err != nil {
		t.Fatalf("GetVersion: %v", err)
	}
	if v.Edition != 2 || v.MigratorCount != 9 || v.SchemaVersion != "2.19.0" {
		t.Fatalf("got %+v", v)
	}
}

func TestClearUpdatingFlag(t *testing.T) {
	path := newTestDB(t, `{"SchemaVersion":"2.19.0"}`)
	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("version"))
		return b.Put([]byte("DB_UPDATING"), []byte("true"))
	})
	if err != nil {
		t.Fatalf("seed updating flag: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	res, err := portainer.ClearUpdatingFlag(path, boltio.WriteOptions{Yes: true, Out: &bytes.Buffer{}})
	if err != nil {
		t.Fatalf("ClearUpdatingFlag: %v", err)
	}
	if !res.Wrote {
		t.Fatal("expected write")
	}

	val, found, err := boltio.Get(path, "version", "DB_UPDATING")
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if string(val) != "false" {
		t.Fatalf("got %q", val)
	}
}
