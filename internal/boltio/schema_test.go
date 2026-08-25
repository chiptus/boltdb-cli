package boltio_test

import (
	"bytes"
	"testing"

	"github.com/chiptus/boltdb-cli/internal/boltio"
)

func TestGetSchemaObjectFields(t *testing.T) {
	path := newTestDB(t)

	res, err := boltio.GetSchema(path, "version", "")
	if err != nil {
		t.Fatalf("GetSchema: %v", err)
	}
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
	path := newTestDB(t)
	_, err := boltio.Put(path, "version", "NESTED",
		[]byte(`{"Outer":{"Inner":42}}`),
		boltio.WriteOptions{Yes: true, Out: &bytes.Buffer{}})
	if err != nil {
		t.Fatalf("seed Put: %v", err)
	}

	res, err := boltio.GetSchema(path, "version", "NESTED")
	if err != nil {
		t.Fatalf("GetSchema: %v", err)
	}
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
	path := newTestDB(t)
	_, err := boltio.Put(path, "version", "ARR",
		[]byte(`{"Tags":["a","b"],"Empty":[]}`),
		boltio.WriteOptions{Yes: true, Out: &bytes.Buffer{}})
	if err != nil {
		t.Fatalf("seed Put: %v", err)
	}

	res, err := boltio.GetSchema(path, "version", "ARR")
	if err != nil {
		t.Fatalf("GetSchema: %v", err)
	}
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
	path := newTestDB(t)
	_, err := boltio.Put(path, "version", "SCALAR", []byte(`42`), boltio.WriteOptions{Yes: true, Out: &bytes.Buffer{}})
	if err != nil {
		t.Fatalf("seed Put: %v", err)
	}

	res, err := boltio.GetSchema(path, "version", "SCALAR")
	if err != nil {
		t.Fatalf("GetSchema: %v", err)
	}
	if res.Shape.Kind != "number" {
		t.Fatalf("got kind %q, want number", res.Shape.Kind)
	}
}

func TestGetSchemaNotJSON(t *testing.T) {
	path := newTestDB(t)
	_, err := boltio.Put(path, "version", "TEXT", []byte("not json"), boltio.WriteOptions{Yes: true, Out: &bytes.Buffer{}})
	if err != nil {
		t.Fatalf("seed Put: %v", err)
	}

	res, err := boltio.GetSchema(path, "version", "TEXT")
	if err != nil {
		t.Fatalf("GetSchema: %v", err)
	}
	if !res.NotJSON {
		t.Fatal("expected NotJSON=true")
	}
	if res.RawByteLen != len("not json") {
		t.Fatalf("got RawByteLen %d, want %d", res.RawByteLen, len("not json"))
	}
}

func TestGetSchemaExplicitKey(t *testing.T) {
	path := newTestDB(t)
	_, err := boltio.Put(path, "version", "OTHER", []byte(`{"X":"y"}`), boltio.WriteOptions{Yes: true, Out: &bytes.Buffer{}})
	if err != nil {
		t.Fatalf("seed Put: %v", err)
	}

	res, err := boltio.GetSchema(path, "version", "OTHER")
	if err != nil {
		t.Fatalf("GetSchema: %v", err)
	}
	if res.SampleKey != "OTHER" {
		t.Fatalf("got sample key %q, want OTHER", res.SampleKey)
	}
	if _, ok := res.Shape.Fields["X"]; !ok {
		t.Fatalf("got %+v, want field X", res.Shape.Fields)
	}
}
