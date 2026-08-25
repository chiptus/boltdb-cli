package boltio_test

import (
	"bytes"
	"testing"

	"github.com/chiptus/boltdb-cli/internal/boltio"
)

func TestGetSchemaNullFieldResolvedViaFallback(t *testing.T) {
	path := newTestDB(t)
	_, err := boltio.Put(path, "version", "A", []byte(`{"Edition":null}`), boltio.WriteOptions{Yes: true, Out: &bytes.Buffer{}})
	if err != nil {
		t.Fatalf("seed Put A: %v", err)
	}
	_, err = boltio.Put(path, "version", "B", []byte(`{"Edition":2}`), boltio.WriteOptions{Yes: true, Out: &bytes.Buffer{}})
	if err != nil {
		t.Fatalf("seed Put B: %v", err)
	}

	res, err := boltio.GetSchema(path, "version", "A")
	if err != nil {
		t.Fatalf("GetSchema: %v", err)
	}
	field := res.Shape.Fields["Edition"]
	if field.Kind != "number" {
		t.Fatalf("got kind %q, want number resolved via fallback", field.Kind)
	}
	if field.ResolvedKey != "B" {
		t.Fatalf("got ResolvedKey %q, want B", field.ResolvedKey)
	}
}

func TestGetSchemaNullFieldUnresolvedWhenNoOtherKeyResolves(t *testing.T) {
	path := newTestDB(t)
	_, err := boltio.Put(path, "version", "A", []byte(`{"Edition":null}`), boltio.WriteOptions{Yes: true, Out: &bytes.Buffer{}})
	if err != nil {
		t.Fatalf("seed Put A: %v", err)
	}
	_, err = boltio.Put(path, "version", "B", []byte(`{"Edition":null}`), boltio.WriteOptions{Yes: true, Out: &bytes.Buffer{}})
	if err != nil {
		t.Fatalf("seed Put B: %v", err)
	}

	res, err := boltio.GetSchema(path, "version", "A")
	if err != nil {
		t.Fatalf("GetSchema: %v", err)
	}
	field := res.Shape.Fields["Edition"]
	if field.Kind != "null" {
		t.Fatalf("got kind %q, want null (unresolved)", field.Kind)
	}
	if field.ResolvedKey != "" {
		t.Fatalf("expected no ResolvedKey, got %q", field.ResolvedKey)
	}
}

func TestGetSchemaMultipleAmbiguousFieldsResolvedInOnePass(t *testing.T) {
	path := newTestDB(t)
	_, err := boltio.Put(path, "version", "A", []byte(`{"Edition":null,"Tags":[]}`), boltio.WriteOptions{Yes: true, Out: &bytes.Buffer{}})
	if err != nil {
		t.Fatalf("seed Put A: %v", err)
	}
	_, err = boltio.Put(path, "version", "B", []byte(`{"Edition":2,"Tags":null}`), boltio.WriteOptions{Yes: true, Out: &bytes.Buffer{}})
	if err != nil {
		t.Fatalf("seed Put B: %v", err)
	}
	_, err = boltio.Put(path, "version", "C", []byte(`{"Edition":null,"Tags":["x"]}`), boltio.WriteOptions{Yes: true, Out: &bytes.Buffer{}})
	if err != nil {
		t.Fatalf("seed Put C: %v", err)
	}

	res, err := boltio.GetSchema(path, "version", "A")
	if err != nil {
		t.Fatalf("GetSchema: %v", err)
	}

	edition := res.Shape.Fields["Edition"]
	if edition.Kind != "number" || edition.ResolvedKey != "B" {
		t.Fatalf("got Edition %+v, want number resolved from B", edition)
	}
	tags := res.Shape.Fields["Tags"]
	if tags.Kind != "array" || tags.Elem == nil || tags.Elem.Kind != "string" || tags.ResolvedKey != "C" {
		t.Fatalf("got Tags %+v, want array<string> resolved from C", tags)
	}
}
