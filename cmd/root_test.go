package cmd_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chiptus/boltdb-cli/cmd"
	bolt "go.etcd.io/bbolt"
)

// seedDB creates a temp bbolt file with a single bucket/key/value seeded,
// the setup shared by every test that needs a pre-populated database.
func seedDB(t *testing.T, bucket string, key, value []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")

	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(bucket))
		if err != nil {
			return err
		}
		return b.Put(key, value)
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return path
}

// newEmptyTestDB creates a temp bbolt file with a single empty bucket.
func newEmptyTestDB(t *testing.T, bucket string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")

	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(bucket))
		return err
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return path
}

func newTestDB(t *testing.T) string {
	t.Helper()
	return seedDB(t, "greeting", []byte("hello"), []byte("world"))
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

	out, err := run(t, "", "list-buckets", "--db", path)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if strings.TrimSpace(out) != "greeting" {
		t.Fatalf("got %q", out)
	}
}

func TestListKeysCmd(t *testing.T) {
	path := newTestDB(t)

	out, err := run(t, "", "list-keys", "--db", path, "greeting")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if strings.TrimSpace(out) != "hello" {
		t.Fatalf("got %q", out)
	}
}

func TestListKeysCmdBase64ForBinaryKeys(t *testing.T) {
	// Mimics Portainer's NextSequence()-derived binary keys.
	path := seedDB(t, "teams", []byte{0, 0, 0, 0, 0, 0, 0, 1}, []byte("team-1"))

	out, err := run(t, "", "list-keys", "--format", "base64", "--db", path, "teams")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if strings.TrimSpace(out) != "AAAAAAAAAAE=" {
		t.Fatalf("got %q", out)
	}
}

func TestListKeysCmdUint64BEForSequenceKeys(t *testing.T) {
	path := seedDB(t, "teams", []byte{0, 0, 0, 0, 0, 0, 0, 1}, []byte("team-1"))

	out, err := run(t, "", "list-keys", "--format", "uint64-be", "--db", path, "teams")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if strings.TrimSpace(out) != "1" {
		t.Fatalf("got %q", out)
	}
}

func TestGetCmdUint64BEKey(t *testing.T) {
	path := seedDB(t, "users", []byte{0, 0, 0, 0, 0, 0, 0, 1}, []byte("admin"))

	out, err := run(t, "", "get", "--key-format", "uint64-be", "--db", path, "users", "1")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if strings.TrimSpace(out) != "admin" {
		t.Fatalf("got %q", out)
	}
}

func TestListBucketsCmdUsesEnvVarWhenNoFlagOrArg(t *testing.T) {
	path := newTestDB(t)
	t.Setenv("BOLTDB_CLI_PATH", path)

	out, err := run(t, "", "list-buckets")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if strings.TrimSpace(out) != "greeting" {
		t.Fatalf("got %q", out)
	}
}

func TestListBucketsCmdDbFlagOverridesEnvVar(t *testing.T) {
	t.Setenv("BOLTDB_CLI_PATH", "/nonexistent/wrong.db")
	path := newTestDB(t)

	out, err := run(t, "", "list-buckets", "--db", path)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if strings.TrimSpace(out) != "greeting" {
		t.Fatalf("got %q", out)
	}
}

func TestListBucketsCmdErrorsWithNoDbPathSource(t *testing.T) {
	t.Setenv("BOLTDB_CLI_PATH", "")

	_, err := run(t, "", "list-buckets")
	if err == nil {
		t.Fatal("expected an error when no --db flag or BOLTDB_CLI_PATH is set")
	}
}

func TestGetCmd(t *testing.T) {
	path := newTestDB(t)

	out, err := run(t, "", "get", "--db", path, "greeting", "hello")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if strings.TrimSpace(out) != "world" {
		t.Fatalf("got %q", out)
	}
}

func TestGetCmdBase64(t *testing.T) {
	path := newTestDB(t)

	out, err := run(t, "", "get", "--format", "base64", "--db", path, "greeting", "hello")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if strings.TrimSpace(out) != "d29ybGQ=" {
		t.Fatalf("got %q", out)
	}
}

func TestPutCmdDryRunDoesNotWrite(t *testing.T) {
	path := newTestDB(t)

	_, err := run(t, "", "put", "--dry-run", "--db", path, "greeting", "hello", "changed")
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	out, err := run(t, "", "get", "--db", path, "greeting", "hello")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if strings.TrimSpace(out) != "world" {
		t.Fatalf("dry-run should not have written, got %q", out)
	}
}

func TestPutCmdWithYesWrites(t *testing.T) {
	path := newTestDB(t)

	_, err := run(t, "", "put", "--yes", "--db", path, "greeting", "hello", "changed")
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	out, err := run(t, "", "get", "--db", path, "greeting", "hello")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if strings.TrimSpace(out) != "changed" {
		t.Fatalf("got %q", out)
	}
}

func TestPutCmdBase64Roundtrip(t *testing.T) {
	path := newTestDB(t)

	_, err := run(t, "", "put", "--yes", "--format", "base64", "--db", path, "greeting", "hello", "aGVsbG8=")
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	out, err := run(t, "", "get", "--db", path, "greeting", "hello")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if strings.TrimSpace(out) != "hello" {
		t.Fatalf("got %q", out)
	}
}

func TestPutCmdPromptAbortsWithoutYes(t *testing.T) {
	path := newTestDB(t)

	_, err := run(t, "n\n", "put", "--db", path, "greeting", "hello", "changed")
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	out, err := run(t, "", "get", "--db", path, "greeting", "hello")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if strings.TrimSpace(out) != "world" {
		t.Fatalf("expected abort to leave value unchanged, got %q", out)
	}
}

func newTestDBWithJSON(t *testing.T) string {
	t.Helper()
	return seedDB(t, "version", []byte("VERSION"), []byte(`{"SchemaVersion":"2.19.0","Edition":1}`))
}

func TestPatchCmdMergesField(t *testing.T) {
	path := newTestDBWithJSON(t)

	_, err := run(t, "", "patch", "--yes", "--db", path, "version", "VERSION", `{"SchemaVersion":"2.20.0"}`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	out, err := run(t, "", "get", "--db", path, "version", "VERSION")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if strings.TrimSpace(out) != `{"SchemaVersion":"2.20.0","Edition":1}` {
		t.Fatalf("got %q", out)
	}
}

func TestPatchCmdDryRunDoesNotWrite(t *testing.T) {
	path := newTestDBWithJSON(t)

	_, err := run(t, "", "patch", "--dry-run", "--db", path, "version", "VERSION", `{"SchemaVersion":"2.20.0"}`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	out, err := run(t, "", "get", "--db", path, "version", "VERSION")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if strings.TrimSpace(out) != `{"SchemaVersion":"2.19.0","Edition":1}` {
		t.Fatalf("dry-run should not have written, got %q", out)
	}
}

func TestPatchCmdMissingKeyErrors(t *testing.T) {
	path := newTestDBWithJSON(t)

	_, err := run(t, "", "patch", "--yes", "--db", path, "version", "NOPE", `{"A":1}`)
	if err == nil {
		t.Fatal("expected error for missing key")
	}
}

func TestPatchCmdNonObjectFragmentErrors(t *testing.T) {
	path := newTestDBWithJSON(t)

	_, err := run(t, "", "patch", "--yes", "--db", path, "version", "VERSION", `[1,2,3]`)
	if err == nil {
		t.Fatal("expected error for non-object fragment")
	}
}

func TestPatchCmdMissingKeyErrorReportsRawKeyNotDecodedBytes(t *testing.T) {
	path := newEmptyTestDB(t, "teams")

	_, err := run(t, "", "patch", "--yes", "--key-format", "uint64-be", "--db", path, "teams", "1", `{"A":1}`)
	if err == nil {
		t.Fatal("expected error for missing key")
	}
	if !strings.Contains(err.Error(), `key "1"`) {
		t.Fatalf("expected error to report the raw typed key %q, got %q", "1", err.Error())
	}
}

func TestPatchCmdKeyFormatUint64BE(t *testing.T) {
	path := seedDB(t, "teams", []byte{0, 0, 0, 0, 0, 0, 0, 1}, []byte(`{"Name":"team-1"}`))

	_, err := run(t, "", "patch", "--yes", "--key-format", "uint64-be", "--db", path, "teams", "1", `{"Name":"team-one"}`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	out, err := run(t, "", "get", "--key-format", "uint64-be", "--db", path, "teams", "1")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if strings.TrimSpace(out) != `{"Name":"team-one"}` {
		t.Fatalf("got %q", out)
	}
}
