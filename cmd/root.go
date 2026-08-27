// Package cmd wires the boltdb-cli command tree. Generic bucket/key
// commands sit at the top level; commands that know Portainer's specific
// schema live under the "portainer" subcommand group.
package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"sort"
	"strings"

	"github.com/chiptus/boltdb-cli/internal/boltio"
	"github.com/chiptus/boltdb-cli/internal/valuefmt"
	"github.com/spf13/cobra"
	bolt "go.etcd.io/bbolt"
)

// dbPathEnvVar lets a database path be set once per shell session instead
// of being passed to every invocation.
const dbPathEnvVar = "BOLTDB_CLI_PATH"

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

const formatFlagUsage = "value format: text, base64, hex, or uint64-be"
const keyFormatFlagUsage = "key format: text, base64, hex, or uint64-be (Portainer's NextSequence() keys); guessed from the bucket's existing keys if unset"

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

// writeFlags holds the --dry-run/--yes flags shared by every command that
// writes through boltio.Put, and builds the WriteOptions they map to.
type writeFlags struct {
	dryRun bool
	yes    bool
}

func (w *writeFlags) register(c *cobra.Command) {
	c.Flags().BoolVar(&w.dryRun, "dry-run", false, "preview the change without writing")
	c.Flags().BoolVarP(&w.yes, "yes", "f", false, "skip the confirmation prompt")
}

func (w *writeFlags) writeOptions(cmd *cobra.Command) boltio.WriteOptions {
	return boltio.WriteOptions{
		DryRun: w.dryRun,
		Yes:    w.yes,
		In:     cmd.InOrStdin(),
		Out:    cmd.OutOrStdout(),
	}
}

// buildVersion returns the module version go install/go build recorded in
// the binary (e.g. "v0.1.0", or "(devel)" for a local, untagged build).
func buildVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "unknown"
}

// NewRootCmd builds the boltdb-cli command tree.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:     "boltdb-cli",
		Short:   "Inspect and patch bbolt database files",
		Version: buildVersion(),
	}
	root.PersistentFlags().String("db", "", fmt.Sprintf("path to the bbolt database file (falls back to the %s env var if unset)", dbPathEnvVar))

	root.AddCommand(
		newListBucketsCmd(),
		newListKeysCmd(),
		newGetCmd(),
		newPutCmd(),
		newPatchCmd(),
		newGetSchemaCmd(),
		newPortainerCmd(),
	)

	return root
}

func printNames(cmd *cobra.Command, names []string, f valuefmt.Format) error {
	for _, n := range names {
		s, err := valuefmt.Encode([]byte(n), f)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), s); err != nil {
			return err
		}
	}
	return nil
}

func newListBucketsCmd() *cobra.Command {
	var format string

	c := &cobra.Command{
		Use:   "list-buckets",
		Short: "List every bucket in a bbolt file",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := resolveDBPath(cmd)
			if err != nil {
				return err
			}
			f, err := valuefmt.Parse(format)
			if err != nil {
				return err
			}
			db, err := boltio.OpenRead(path)
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()

			var buckets []string
			err = db.View(func(tx *bolt.Tx) error {
				buckets = boltio.ListBuckets(tx)
				return nil
			})
			if err != nil {
				return err
			}
			return printNames(cmd, buckets, f)
		},
	}
	c.Flags().StringVar(&format, "format", "text", formatFlagUsage)
	return c
}

func newListKeysCmd() *cobra.Command {
	var format string

	c := &cobra.Command{
		Use:   "list-keys <bucket>",
		Short: "List every key in a bucket",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := resolveDBPath(cmd)
			if err != nil {
				return err
			}
			f, err := valuefmt.Parse(format)
			if err != nil {
				return err
			}
			db, err := boltio.OpenRead(path)
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()

			var keys []string
			err = db.View(func(tx *bolt.Tx) error {
				var err error
				keys, err = boltio.ListKeys(tx, args[0])
				return err
			})
			if err != nil {
				return err
			}
			return printNames(cmd, keys, f)
		},
	}
	c.Flags().StringVar(&format, "format", "text", formatFlagUsage)
	return c
}

func newGetCmd() *cobra.Command {
	var format, keyFormat string

	c := &cobra.Command{
		Use:   "get <bucket> <key>",
		Short: "Print the value stored at bucket/key",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := resolveDBPath(cmd)
			if err != nil {
				return err
			}
			bucket, rawKey := args[0], args[1]

			db, err := boltio.OpenRead(path)
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()

			var val []byte
			err = db.View(func(tx *bolt.Tx) error {
				kf, err := resolveKeyFormat(tx, bucket, keyFormat)
				if err != nil {
					return err
				}
				key, err := valuefmt.Decode(rawKey, kf)
				if err != nil {
					return fmt.Errorf("decode key: %w", err)
				}

				v, found := boltio.Get(tx, bucket, string(key))
				if !found {
					return fmt.Errorf("no value at bucket %q key %q", bucket, rawKey)
				}
				val = v
				return nil
			})
			if err != nil {
				return err
			}
			vf, err := valuefmt.Parse(format)
			if err != nil {
				return err
			}
			s, err := valuefmt.Encode(val, vf)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), s)
			return err
		},
	}
	c.Flags().StringVar(&format, "format", "text", formatFlagUsage)
	c.Flags().StringVar(&keyFormat, "key-format", "", keyFormatFlagUsage)
	return c
}

func newPutCmd() *cobra.Command {
	var format, keyFormat string
	wf := &writeFlags{}

	c := &cobra.Command{
		Use:   "put <bucket> <key> <value>",
		Short: "Write a value at bucket/key",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := resolveDBPath(cmd)
			if err != nil {
				return err
			}
			bucket, rawKey, rawValue := args[0], args[1], args[2]

			db, err := boltio.OpenWrite(path)
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()

			var kf valuefmt.Format
			err = db.View(func(tx *bolt.Tx) error {
				var err error
				kf, err = resolveKeyFormat(tx, bucket, keyFormat)
				return err
			})
			if err != nil {
				return err
			}
			key, err := valuefmt.Decode(rawKey, kf)
			if err != nil {
				return fmt.Errorf("decode key: %w", err)
			}
			vf, err := valuefmt.Parse(format)
			if err != nil {
				return err
			}
			value, err := valuefmt.Decode(rawValue, vf)
			if err != nil {
				return fmt.Errorf("decode value: %w", err)
			}

			opts := wf.writeOptions(cmd)
			opts.Render = func(v []byte) (string, error) { return valuefmt.Encode(v, vf) }
			_, err = boltio.Put(db, bucket, string(key), value, opts)
			return err
		},
	}
	c.Flags().StringVar(&format, "format", "text", formatFlagUsage)
	c.Flags().StringVar(&keyFormat, "key-format", "", keyFormatFlagUsage)
	wf.register(c)
	return c
}

const schemaFormatFlagUsage = "output format: text (human-readable tree) or json"

func newGetSchemaCmd() *cobra.Command {
	var key, keyFormat, format string

	c := &cobra.Command{
		Use:   "get-schema <bucket>",
		Short: "Infer the field shape of a sample value in a bucket",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := resolveDBPath(cmd)
			if err != nil {
				return err
			}
			bucket := args[0]

			db, err := boltio.OpenRead(path)
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()

			var result boltio.GetSchemaResult
			err = db.View(func(tx *bolt.Tx) error {
				var sampleKey string
				if cmd.Flags().Changed("key") {
					kf, err := resolveKeyFormat(tx, bucket, keyFormat)
					if err != nil {
						return err
					}
					decoded, err := valuefmt.Decode(key, kf)
					if err != nil {
						return fmt.Errorf("decode key: %w", err)
					}
					sampleKey = string(decoded)
				}

				var err error
				result, err = boltio.GetSchema(tx, bucket, sampleKey)
				return err
			})
			if err != nil {
				return err
			}
			return printSchema(cmd, result, format)
		},
	}
	c.Flags().StringVar(&key, "key", "", "sample this specific key instead of the bucket's first key")
	c.Flags().StringVar(&keyFormat, "key-format", "", keyFormatFlagUsage)
	c.Flags().StringVar(&format, "format", "text", schemaFormatFlagUsage)
	return c
}

// printSchema renders a GetSchemaResult in the requested format ("text" for
// a human-readable tree, or "json" for a machine-parseable shape
// description).
func printSchema(cmd *cobra.Command, result boltio.GetSchemaResult, format string) error {
	out := cmd.OutOrStdout()

	switch format {
	case "", "text":
		if result.NotJSON {
			_, err := fmt.Fprintf(out, "bucket %q key %q: not JSON — %d bytes, raw text/binary\n", result.Bucket, result.SampleKey, result.RawByteLen)
			return err
		}
		if _, err := fmt.Fprintf(out, "bucket %q (inferred from key %q, 1 sample)\n", result.Bucket, result.SampleKey); err != nil {
			return err
		}
		return printShapeTree(out, result.Shape, 0)
	case "json":
		if result.NotJSON {
			return json.NewEncoder(out).Encode(map[string]any{
				"notJSON":    true,
				"bucket":     result.Bucket,
				"sampleKey":  result.SampleKey,
				"rawByteLen": result.RawByteLen,
			})
		}
		return json.NewEncoder(out).Encode(shapeToJSON(result.Shape))
	default:
		return fmt.Errorf("unknown format %q (want text or json)", format)
	}
}

// shapeTypeLabel renders s's type as a compact label, e.g. "string" or
// "array<object>". Objects are not expanded here — use printShapeTree or
// shapeToJSON for their nested fields.
func shapeTypeLabel(s boltio.Shape) string {
	if s.Kind != "array" {
		return s.Kind
	}
	if s.Elem == nil {
		return "array<empty>"
	}
	return "array<" + shapeTypeLabel(*s.Elem) + ">"
}

func sortedFieldNames(fields map[string]boltio.Shape) []string {
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// printShapeTree writes s as an indented human-readable tree. Only object
// fields are recursed into; array element shapes are rendered as a
// compact label (e.g. "array<object>"), not expanded.
func printShapeTree(out io.Writer, s boltio.Shape, depth int) error {
	indent := strings.Repeat("  ", depth)
	if s.Kind != "object" {
		_, err := fmt.Fprintf(out, "%s%s\n", indent, shapeTypeLabelWithProvenance(s))
		return err
	}
	for _, name := range sortedFieldNames(s.Fields) {
		field := s.Fields[name]
		if field.Kind == "object" {
			if _, err := fmt.Fprintf(out, "%s%s:\n", indent, name); err != nil {
				return err
			}
			if err := printShapeTree(out, field, depth+1); err != nil {
				return err
			}
			continue
		}
		if _, err := fmt.Fprintf(out, "%s%s: %s\n", indent, name, shapeTypeLabelWithProvenance(field)); err != nil {
			return err
		}
	}
	return nil
}

// shapeTypeLabelWithProvenance appends a note when s's type was resolved
// via the fallback scan rather than the primary sample.
func shapeTypeLabelWithProvenance(s boltio.Shape) string {
	label := shapeTypeLabel(s)
	if s.ResolvedKey != "" {
		return fmt.Sprintf("%s (resolved from key %q, ambiguous in sample)", label, s.ResolvedKey)
	}
	return label
}

// shapeToJSON renders s as a value suitable for JSON encoding: nested
// objects become nested JSON objects, everything else (including arrays)
// becomes its compact type label string.
func shapeToJSON(s boltio.Shape) any {
	if s.Kind != "object" {
		return shapeTypeLabel(s)
	}
	m := make(map[string]any, len(s.Fields))
	for name, field := range s.Fields {
		m[name] = shapeToJSON(field)
	}
	return m
}

func newPatchCmd() *cobra.Command {
	var keyFormat string
	wf := &writeFlags{}

	c := &cobra.Command{
		Use:   "patch <bucket> <key> <json-fragment>",
		Short: "Merge a JSON fragment into the JSON object stored at bucket/key (RFC 7396)",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := resolveDBPath(cmd)
			if err != nil {
				return err
			}
			bucket, rawKey, fragment := args[0], args[1], args[2]

			db, err := boltio.OpenWrite(path)
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()

			var key []byte
			err = db.View(func(tx *bolt.Tx) error {
				kf, err := resolveKeyFormat(tx, bucket, keyFormat)
				if err != nil {
					return err
				}
				decoded, err := valuefmt.Decode(rawKey, kf)
				if err != nil {
					return fmt.Errorf("decode key: %w", err)
				}
				key = decoded

				// Check existence here (as get does) so a missing-key error
				// reports the raw typed key, not the decoded bytes
				// boltio.Patch operates on internally.
				if _, found := boltio.Get(tx, bucket, string(key)); !found {
					return fmt.Errorf("no value at bucket %q key %q", bucket, rawKey)
				}
				return nil
			})
			if err != nil {
				return err
			}

			opts := wf.writeOptions(cmd)
			// No --format flag: patch always operates on JSON, so the
			// dry-run preview renders as plain text (JSON is text).
			_, err = boltio.Patch(db, bucket, string(key), []byte(fragment), opts)
			return err
		},
	}
	c.Flags().StringVar(&keyFormat, "key-format", "", keyFormatFlagUsage)
	wf.register(c)
	return c
}
