package boltio_test

import (
	"bytes"
	"testing"

	"github.com/chiptus/boltdb-cli/internal/boltio"
	bolt "go.etcd.io/bbolt"
)

func getSchema(t *testing.T, db *bolt.DB, bucket, key string) boltio.GetSchemaResult {
	t.Helper()
	var res boltio.GetSchemaResult
	err := db.View(func(tx *bolt.Tx) error {
		var err error
		res, err = boltio.GetSchema(tx, bucket, key)
		return err
	})
	if err != nil {
		t.Fatalf("GetSchema: %v", err)
	}
	return res
}

func TestGetSchemaObjectFields(t *testing.T) {
	db := newTestDB(t)

	res := getSchema(t, db, "version", "")
	if res.SampleKey != "VERSION" {
		t.Fatalf("got sample key %q, want VERSION", res.SampleKey)
	}
	if res.Shape.Kind != "object" {
		t.Fatalf("got kind %q, want object", res.Shape.Kind)
	}
	field, ok := res.Shape.Fields["SchemaVersion"]
	if !ok || field.Kind != "string" {
		t.Fatalf("got fields %+v, want SchemaVersion: string", res.Shape.Fields)
	}
}

func TestGetSchemaNestedObject(t *testing.T) {
	db := newTestDB(t)
	_, err := boltio.Put(db, "version", "NESTED",
		[]byte(`{"Outer":{"Inner":42}}`),
		boltio.WriteOptions{Yes: true, Out: &bytes.Buffer{}})
	if err != nil {
		t.Fatalf("seed Put: %v", err)
	}

	res := getSchema(t, db, "version", "NESTED")
	outer, ok := res.Shape.Fields["Outer"]
	if !ok || outer.Kind != "object" {
		t.Fatalf("got %+v, want Outer: object", res.Shape.Fields)
	}
	inner, ok := outer.Fields["Inner"]
	if !ok || inner.Kind != "number" {
		t.Fatalf("got %+v, want Inner: number", outer.Fields)
	}
}

func TestGetSchemaArrayField(t *testing.T) {
	db := newTestDB(t)
	_, err := boltio.Put(db, "version", "ARR",
		[]byte(`{"Tags":["a","b"],"Empty":[]}`),
		boltio.WriteOptions{Yes: true, Out: &bytes.Buffer{}})
	if err != nil {
		t.Fatalf("seed Put: %v", err)
	}

	res := getSchema(t, db, "version", "ARR")
	tags := res.Shape.Fields["Tags"]
	if tags.Kind != "array" || tags.Elem == nil || tags.Elem.Kind != "string" {
		t.Fatalf("got %+v, want array<string>", tags)
	}
	empty := res.Shape.Fields["Empty"]
	if empty.Kind != "array" || empty.Elem != nil {
		t.Fatalf("got %+v, want array<empty>", empty)
	}
}

func TestGetSchemaNonObjectTopLevel(t *testing.T) {
	db := newTestDB(t)
	_, err := boltio.Put(db, "version", "SCALAR", []byte(`42`), boltio.WriteOptions{Yes: true, Out: &bytes.Buffer{}})
	if err != nil {
		t.Fatalf("seed Put: %v", err)
	}

	res := getSchema(t, db, "version", "SCALAR")
	if res.Shape.Kind != "number" {
		t.Fatalf("got kind %q, want number", res.Shape.Kind)
	}
}

func TestGetSchemaNotJSON(t *testing.T) {
	db := newTestDB(t)
	_, err := boltio.Put(db, "version", "TEXT", []byte("not json"), boltio.WriteOptions{Yes: true, Out: &bytes.Buffer{}})
	if err != nil {
		t.Fatalf("seed Put: %v", err)
	}

	res := getSchema(t, db, "version", "TEXT")
	if !res.NotJSON {
		t.Fatal("expected NotJSON=true")
	}
	if res.RawByteLen != len("not json") {
		t.Fatalf("got RawByteLen %d, want %d", res.RawByteLen, len("not json"))
	}
}

func TestGetSchemaExplicitKey(t *testing.T) {
	db := newTestDB(t)
	_, err := boltio.Put(db, "version", "OTHER", []byte(`{"X":"y"}`), boltio.WriteOptions{Yes: true, Out: &bytes.Buffer{}})
	if err != nil {
		t.Fatalf("seed Put: %v", err)
	}

	res := getSchema(t, db, "version", "OTHER")
	if res.SampleKey != "OTHER" {
		t.Fatalf("got sample key %q, want OTHER", res.SampleKey)
	}
	if _, ok := res.Shape.Fields["X"]; !ok {
		t.Fatalf("got %+v, want field X", res.Shape.Fields)
	}
}
