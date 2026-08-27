package cmd

import (
	"fmt"
	"os"

	"github.com/chiptus/boltdb-cli/internal/boltio"
	"github.com/chiptus/boltdb-cli/internal/valuefmt"
	"github.com/spf13/cobra"
	bolt "go.etcd.io/bbolt"
)

// dbPathEnvVar lets a database path be set once per shell session instead
// of being passed to every invocation.
const dbPathEnvVar = "BOLTDB_CLI_PATH"

const keyFormatFlagUsage = "key format: text, base64, hex, or uint64-be (Portainer's NextSequence() keys); guessed from the bucket's existing keys if unset"

// resolveDBPath returns the database path a command should use: the --db
// flag if set, else the BOLTDB_CLI_PATH env var.
func resolveDBPath(cmd *cobra.Command) (string, error) {
	if dbFlag, _ := cmd.Flags().GetString("db"); dbFlag != "" {
		return dbFlag, nil
	}
	if env := os.Getenv(dbPathEnvVar); env != "" {
		return env, nil
	}
	return "", fmt.Errorf("no database path: use --db or set %s", dbPathEnvVar)
}

// openReadDB resolves the database path from cmd's --db flag/env var and
// opens it read-only.
func openReadDB(cmd *cobra.Command) (*bolt.DB, error) {
	path, err := resolveDBPath(cmd)
	if err != nil {
		return nil, err
	}
	return boltio.OpenRead(path)
}

// openWriteDB resolves the database path from cmd's --db flag/env var and
// opens it read-write.
func openWriteDB(cmd *cobra.Command) (*bolt.DB, error) {
	path, err := resolveDBPath(cmd)
	if err != nil {
		return nil, err
	}
	return boltio.OpenWrite(path)
}

// resolveKeyFormat returns keyFormat as given if the user set --key-format
// to something, otherwise it guesses a format from the bucket's existing
// keys (see boltio.GuessKeyFormat), falling back to valuefmt.Text if the
// bucket is empty, unreadable, or its keys don't clearly fit one format.
func resolveKeyFormat(tx *bolt.Tx, bucket, keyFormat string) (valuefmt.Format, error) {
	if keyFormat != "" {
		return valuefmt.Parse(keyFormat)
	}
	guess, err := boltio.GuessKeyFormat(tx, bucket)
	if err != nil || guess == valuefmt.Format("") {
		return valuefmt.Text, nil
	}
	return guess, nil
}

// resolveKey resolves keyFormat (see resolveKeyFormat) and decodes rawKey
// with it, the format-then-decode sequence every command needs before
// looking up or writing a key.
func resolveKey(tx *bolt.Tx, bucket, rawKey, keyFormat string) ([]byte, error) {
	kf, err := resolveKeyFormat(tx, bucket, keyFormat)
	if err != nil {
		return nil, err
	}
	key, err := valuefmt.Decode(rawKey, kf)
	if err != nil {
		return nil, fmt.Errorf("decode key: %w", err)
	}
	return key, nil
}
