package boltio_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chiptus/boltdb-cli/internal/boltio"
	bolt "go.etcd.io/bbolt"
)

func newTestDB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte("version"))
		if err != nil {
			return err
		}
		return b.Put([]byte("VERSION"), []byte(`{"SchemaVersion":"2.19.0"}`))
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return path
}

func TestListBuckets(t *testing.T) {
	path := newTestDB(t)

	buckets, err := boltio.ListBuckets(path)
	if err != nil {
		t.Fatalf("ListBuckets: %v", err)
	}
	if len(buckets) != 1 || buckets[0] != "version" {
		t.Fatalf("got %v, want [version]", buckets)
	}
}

func TestListKeys(t *testing.T) {
	path := newTestDB(t)

	keys, err := boltio.ListKeys(path, "version")
	if err != nil {
		t.Fatalf("ListKeys: %v", err)
	}
	if len(keys) != 1 || keys[0] != "VERSION" {
		t.Fatalf("got %v, want [VERSION]", keys)
	}
}

func TestListKeysMissingBucket(t *testing.T) {
	path := newTestDB(t)

	_, err := boltio.ListKeys(path, "does-not-exist")
	if err == nil {
		t.Fatal("expected error for missing bucket")
	}
}

func TestGet(t *testing.T) {
	path := newTestDB(t)

	val, found, err := boltio.Get(path, "version", "VERSION")
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
	path := newTestDB(t)

	_, found, err := boltio.Get(path, "version", "NOPE")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if found {
		t.Fatal("expected found=false")
	}
}

func TestPutDryRunDoesNotWriteOrBackup(t *testing.T) {
	path := newTestDB(t)
	out := &bytes.Buffer{}

	res, err := boltio.Put(path, "version", "VERSION", []byte(`{"SchemaVersion":"2.20.0"}`), boltio.WriteOptions{
		DryRun: true,
		Out:    out,
	})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if res.Wrote {
		t.Fatal("expected dry-run to not write")
	}
	if res.BackupPath != "" {
		t.Fatal("expected dry-run to not create a backup")
	}

	val, _, err := boltio.Get(path, "version", "VERSION")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(val) != `{"SchemaVersion":"2.19.0"}` {
		t.Fatalf("dry-run mutated the file: got %q", val)
	}

	if !strings.Contains(out.String(), "2.19.0") || !strings.Contains(out.String(), "2.20.0") {
		t.Fatalf("expected preview to mention old and new values, got %q", out.String())
	}
}

func TestPutWithYesWritesAndBackups(t *testing.T) {
	path := newTestDB(t)
	out := &bytes.Buffer{}

	res, err := boltio.Put(path, "version", "VERSION", []byte(`{"SchemaVersion":"2.20.0"}`), boltio.WriteOptions{
		Yes: true,
		Out: out,
	})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if !res.Wrote {
		t.Fatal("expected write to happen")
	}
	if res.BackupPath == "" {
		t.Fatal("expected a backup path")
	}

	backupData, err := os.ReadFile(res.BackupPath)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if !strings.Contains(string(backupData), "") {
		t.Fatalf("unexpected backup contents")
	}

	val, _, err := boltio.Get(path, "version", "VERSION")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(val) != `{"SchemaVersion":"2.20.0"}` {
		t.Fatalf("got %q", val)
	}

	backupVal, _, err := boltio.Get(res.BackupPath, "version", "VERSION")
	if err != nil {
		t.Fatalf("Get on backup: %v", err)
	}
	if string(backupVal) != `{"SchemaVersion":"2.19.0"}` {
		t.Fatalf("backup should hold the pre-write value, got %q", backupVal)
	}
}

func TestPutPromptsAndAbortsOnNo(t *testing.T) {
	path := newTestDB(t)
	out := &bytes.Buffer{}
	in := strings.NewReader("n\n")

	res, err := boltio.Put(path, "version", "VERSION", []byte(`{"SchemaVersion":"2.20.0"}`), boltio.WriteOptions{
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

	val, _, _ := boltio.Get(path, "version", "VERSION")
	if string(val) != `{"SchemaVersion":"2.19.0"}` {
		t.Fatalf("aborted write mutated the file: got %q", val)
	}
}

func TestPutPromptsAndWritesOnYes(t *testing.T) {
	path := newTestDB(t)
	out := &bytes.Buffer{}
	in := strings.NewReader("y\n")

	res, err := boltio.Put(path, "version", "VERSION", []byte(`{"SchemaVersion":"2.20.0"}`), boltio.WriteOptions{
		In:  in,
		Out: out,
	})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if !res.Wrote {
		t.Fatal("expected confirmed 'y' to write")
	}

	val, _, _ := boltio.Get(path, "version", "VERSION")
	if string(val) != `{"SchemaVersion":"2.20.0"}` {
		t.Fatalf("got %q", val)
	}
}

func TestPutDryRunPreviewIsBase64WhenRequested(t *testing.T) {
	path := newTestDB(t)
	out := &bytes.Buffer{}

	binary := []byte{0xff, 0x00, 0xfe}
	_, err := boltio.Put(path, "version", "BINARY", binary, boltio.WriteOptions{
		DryRun: true,
		Format: "base64",
		Out:    out,
	})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	if !strings.Contains(out.String(), "/wD+") { // base64 of 0xff 0x00 0xfe
		t.Fatalf("expected base64-encoded preview, got %q", out.String())
	}
}

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

func TestGetSchemaMissingBucketErrors(t *testing.T) {
	path := newTestDB(t)

	_, err := boltio.GetSchema(path, "does-not-exist", "")
	if err == nil {
		t.Fatal("expected error for missing bucket")
	}
}

func TestGetSchemaEmptyBucketErrors(t *testing.T) {
	path := newTestDB(t)
	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("empty-bucket"))
		return err
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	_, err = boltio.GetSchema(path, "empty-bucket", "")
	if err == nil {
		t.Fatal("expected error for empty bucket")
	}
}

func TestPutCreatesBucketIfMissing(t *testing.T) {
	path := newTestDB(t)

	res, err := boltio.Put(path, "new-bucket", "key1", []byte("hello"), boltio.WriteOptions{Yes: true, Out: &bytes.Buffer{}})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if !res.Wrote {
		t.Fatal("expected write")
	}

	val, found, err := boltio.Get(path, "new-bucket", "key1")
	if err != nil || !found {
		t.Fatalf("Get: found=%v err=%v", found, err)
	}
	if string(val) != "hello" {
		t.Fatalf("got %q", val)
	}
}
