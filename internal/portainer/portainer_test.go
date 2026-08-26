package portainer_test

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/chiptus/boltdb-cli/internal/boltio"
	"github.com/chiptus/boltdb-cli/internal/portainer"
	bolt "go.etcd.io/bbolt"
)

func newTestDB(t *testing.T, versionJSON string) *bolt.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")

	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

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
	return db
}

func getVersion(t *testing.T, db *bolt.DB) portainer.Version {
	t.Helper()
	var v portainer.Version
	err := db.View(func(tx *bolt.Tx) error {
		var err error
		v, err = portainer.GetVersion(tx)
		return err
	})
	if err != nil {
		t.Fatalf("GetVersion: %v", err)
	}
	return v
}

func TestGetVersion(t *testing.T) {
	db := newTestDB(t, `{"SchemaVersion":"2.19.0","MigratorCount":5,"Edition":1,"InstanceID":"abc"}`)

	v := getVersion(t, db)
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
	t.Cleanup(func() { _ = db.Close() })

	err = db.View(func(tx *bolt.Tx) error {
		_, err := portainer.GetVersion(tx)
		return err
	})
	if err == nil {
		t.Fatal("expected error when version key is absent")
	}
}

func TestSetVersionUpdatesOnlyRequestedFields(t *testing.T) {
	db := newTestDB(t, `{"SchemaVersion":"2.19.0","MigratorCount":5,"Edition":1,"InstanceID":"abc"}`)
	schemaVersion := "2.20.0"

	res, err := portainer.SetVersion(db, portainer.SetVersionInput{
		SchemaVersion: &schemaVersion,
	}, boltio.WriteOptions{Yes: true, Out: &bytes.Buffer{}})
	if err != nil {
		t.Fatalf("SetVersion: %v", err)
	}
	if !res.Wrote {
		t.Fatal("expected write")
	}

	v := getVersion(t, db)
	if v.SchemaVersion != "2.20.0" {
		t.Fatalf("schema version not updated: %+v", v)
	}
	if v.MigratorCount != 5 || v.Edition != 1 || v.InstanceID != "abc" {
		t.Fatalf("unrelated fields should be preserved, got %+v", v)
	}
}

func TestSetVersionRejectsInvalidSemver(t *testing.T) {
	db := newTestDB(t, `{"SchemaVersion":"2.19.0"}`)
	bad := "not-a-version"

	_, err := portainer.SetVersion(db, portainer.SetVersionInput{
		SchemaVersion: &bad,
	}, boltio.WriteOptions{Yes: true, Out: &bytes.Buffer{}})
	if err == nil {
		t.Fatal("expected error for invalid semver")
	}
}

func TestSetVersionEditionAndMigratorCount(t *testing.T) {
	db := newTestDB(t, `{"SchemaVersion":"2.19.0","MigratorCount":1,"Edition":1,"InstanceID":"abc"}`)
	edition := 2
	migratorCount := 9

	_, err := portainer.SetVersion(db, portainer.SetVersionInput{
		Edition:       &edition,
		MigratorCount: &migratorCount,
	}, boltio.WriteOptions{Yes: true, Out: &bytes.Buffer{}})
	if err != nil {
		t.Fatalf("SetVersion: %v", err)
	}

	v := getVersion(t, db)
	if v.Edition != 2 || v.MigratorCount != 9 || v.SchemaVersion != "2.19.0" {
		t.Fatalf("got %+v", v)
	}
}

func TestClearUpdatingFlag(t *testing.T) {
	db := newTestDB(t, `{"SchemaVersion":"2.19.0"}`)
	err := db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("version"))
		return b.Put([]byte("DB_UPDATING"), []byte("true"))
	})
	if err != nil {
		t.Fatalf("seed updating flag: %v", err)
	}

	res, err := portainer.ClearUpdatingFlag(db, boltio.WriteOptions{Yes: true, Out: &bytes.Buffer{}})
	if err != nil {
		t.Fatalf("ClearUpdatingFlag: %v", err)
	}
	if !res.Wrote {
		t.Fatal("expected write")
	}

	var val []byte
	var found bool
	err = db.View(func(tx *bolt.Tx) error {
		val, found = boltio.Get(tx, "version", "DB_UPDATING")
		return nil
	})
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if string(val) != "false" {
		t.Fatalf("got %q", val)
	}
}
