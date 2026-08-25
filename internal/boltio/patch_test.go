package boltio_test

import (
	"bytes"
	"testing"

	"github.com/chiptus/boltdb-cli/internal/boltio"
)

func TestPatchMergesSingleField(t *testing.T) {
	path := newTestDB(t)

	res, err := boltio.Patch(path, "version", "VERSION", []byte(`{"SchemaVersion":"2.20.0"}`), boltio.WriteOptions{
		Yes: true,
		Out: &bytes.Buffer{},
	})
	if err != nil {
		t.Fatalf("Patch: %v", err)
	}
	if !res.Wrote {
		t.Fatal("expected write")
	}

	val, _, err := boltio.Get(path, "version", "VERSION")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(val) != `{"SchemaVersion":"2.20.0"}` {
		t.Fatalf("got %q", val)
	}
}

func TestPatchMergesNestedFieldsAndDeletesNullFields(t *testing.T) {
	path := newTestDB(t)
	_, err := boltio.Put(path, "version", "VERSION",
		[]byte(`{"SchemaVersion":"2.19.0","Edition":1,"Nested":{"A":1,"B":2}}`),
		boltio.WriteOptions{Yes: true, Out: &bytes.Buffer{}})
	if err != nil {
		t.Fatalf("seed Put: %v", err)
	}

	res, err := boltio.Patch(path, "version", "VERSION",
		[]byte(`{"Edition":null,"Nested":{"B":3,"C":4}}`),
		boltio.WriteOptions{Yes: true, Out: &bytes.Buffer{}})
	if err != nil {
		t.Fatalf("Patch: %v", err)
	}
	if !res.Wrote {
		t.Fatal("expected write")
	}

	val, _, err := boltio.Get(path, "version", "VERSION")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	want := `{"SchemaVersion":"2.19.0","Nested":{"A":1,"B":3,"C":4}}`
	if string(val) != want {
		t.Fatalf("got %q, want %q", val, want)
	}
}

func TestPatchMissingKeyErrors(t *testing.T) {
	path := newTestDB(t)

	_, err := boltio.Patch(path, "version", "NOPE", []byte(`{"A":1}`), boltio.WriteOptions{Yes: true, Out: &bytes.Buffer{}})
	if err == nil {
		t.Fatal("expected error for missing key")
	}
}

func TestPatchNonJSONValueErrors(t *testing.T) {
	path := newTestDB(t)
	_, err := boltio.Put(path, "version", "TEXT", []byte("not json"), boltio.WriteOptions{Yes: true, Out: &bytes.Buffer{}})
	if err != nil {
		t.Fatalf("seed Put: %v", err)
	}

	_, err = boltio.Patch(path, "version", "TEXT", []byte(`{"A":1}`), boltio.WriteOptions{Yes: true, Out: &bytes.Buffer{}})
	if err == nil {
		t.Fatal("expected error for non-JSON stored value")
	}
}

func TestPatchNonObjectValueErrors(t *testing.T) {
	path := newTestDB(t)
	_, err := boltio.Put(path, "version", "ARRAY", []byte(`[1,2,3]`), boltio.WriteOptions{Yes: true, Out: &bytes.Buffer{}})
	if err != nil {
		t.Fatalf("seed Put: %v", err)
	}

	_, err = boltio.Patch(path, "version", "ARRAY", []byte(`{"A":1}`), boltio.WriteOptions{Yes: true, Out: &bytes.Buffer{}})
	if err == nil {
		t.Fatal("expected error for non-object stored value")
	}
}

func TestPatchNonJSONFragmentErrors(t *testing.T) {
	path := newTestDB(t)

	_, err := boltio.Patch(path, "version", "VERSION", []byte(`not json`), boltio.WriteOptions{Yes: true, Out: &bytes.Buffer{}})
	if err == nil {
		t.Fatal("expected error for non-JSON fragment")
	}
}

func TestPatchNonObjectFragmentErrors(t *testing.T) {
	path := newTestDB(t)

	_, err := boltio.Patch(path, "version", "VERSION", []byte(`[1,2,3]`), boltio.WriteOptions{Yes: true, Out: &bytes.Buffer{}})
	if err == nil {
		t.Fatal("expected error for non-object fragment")
	}
}

func TestPatchDryRunDoesNotWriteOrBackup(t *testing.T) {
	path := newTestDB(t)

	res, err := boltio.Patch(path, "version", "VERSION", []byte(`{"SchemaVersion":"2.20.0"}`), boltio.WriteOptions{
		DryRun: true,
		Out:    &bytes.Buffer{},
	})
	if err != nil {
		t.Fatalf("Patch: %v", err)
	}
	if res.Wrote {
		t.Fatal("expected dry-run to not write")
	}
	if res.BackupPath != "" {
		t.Fatal("expected dry-run to not create a backup")
	}

	val, _, _ := boltio.Get(path, "version", "VERSION")
	if string(val) != `{"SchemaVersion":"2.19.0"}` {
		t.Fatalf("dry-run mutated the file: got %q", val)
	}
}
