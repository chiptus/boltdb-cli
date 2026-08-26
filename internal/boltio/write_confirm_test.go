package boltio_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/chiptus/boltdb-cli/internal/boltio"
)

func TestPutPromptsAndAbortsOnNo(t *testing.T) {
	db := newTestDB(t)
	out := &bytes.Buffer{}
	in := strings.NewReader("n\n")

	res, err := boltio.Put(db, "version", "VERSION", []byte(`{"SchemaVersion":"2.20.0"}`), boltio.WriteOptions{
		In:  in,
		Out: out,
	})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if res.Wrote {
		t.Fatal("expected abort on 'n' to not write")
	}
	if res.BackupPath != "" {
		t.Fatal("expected no backup on abort")
	}

	val, _ := getFromDB(t, db, "version", "VERSION")
	if string(val) != `{"SchemaVersion":"2.19.0"}` {
		t.Fatalf("aborted write mutated the file: got %q", val)
	}
}

func TestPutPromptsAndWritesOnYes(t *testing.T) {
	db := newTestDB(t)
	out := &bytes.Buffer{}
	in := strings.NewReader("y\n")

	res, err := boltio.Put(db, "version", "VERSION", []byte(`{"SchemaVersion":"2.20.0"}`), boltio.WriteOptions{
		In:  in,
		Out: out,
	})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if !res.Wrote {
		t.Fatal("expected confirmed 'y' to write")
	}

	val, _ := getFromDB(t, db, "version", "VERSION")
	if string(val) != `{"SchemaVersion":"2.20.0"}` {
		t.Fatalf("got %q", val)
	}
}
