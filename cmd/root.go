// Package cmd wires the boltdb-cli command tree. Generic bucket/key
// commands sit at the top level; commands that know Portainer's specific
// schema live under the "portainer" subcommand group.
package cmd

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/chiptus/boltdb-cli/internal/boltio"
	"github.com/spf13/cobra"
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

// encodeBytes renders b in the given output format. An empty format means
// "text" (raw bytes, printed as-is).
func encodeBytes(b []byte, format string) (string, error) {
	switch format {
	case "", "text":
		return string(b), nil
	case "base64":
		return base64.StdEncoding.EncodeToString(b), nil
	case "hex":
		return hex.EncodeToString(b), nil
	case "uint64-be":
		if len(b) != 8 {
			return "", fmt.Errorf("uint64-be format: value is %d bytes, want 8", len(b))
		}
		return strconv.FormatUint(binary.BigEndian.Uint64(b), 10), nil
	default:
		return "", fmt.Errorf("unknown format %q (want text, base64, hex, or uint64-be)", format)
	}
}

// decodeValue parses s according to format into raw bytes, the inverse of
// encodeBytes, for commands that take a value/key on the command line.
func decodeValue(s string, format string) ([]byte, error) {
	switch format {
	case "", "text":
		return []byte(s), nil
	case "base64":
		return base64.StdEncoding.DecodeString(s)
	case "hex":
		return hex.DecodeString(s)
	case "uint64-be":
		n, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("uint64-be format: %w", err)
		}
		buf := make([]byte, 8)
		binary.BigEndian.PutUint64(buf, n)
		return buf, nil
	default:
		return nil, fmt.Errorf("unknown format %q (want text, base64, hex, or uint64-be)", format)
	}
}

const formatFlagUsage = "value format: text, base64, hex, or uint64-be"
const keyFormatFlagUsage = "key format: text, base64, hex, or uint64-be (Portainer's NextSequence() keys)"

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

// NewRootCmd builds the boltdb-cli command tree.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "boltdb-cli",
		Short: "Inspect and patch bbolt database files",
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

func printNames(cmd *cobra.Command, names []string, format string) error {
	for _, n := range names {
		s, err := encodeBytes([]byte(n), format)
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
			buckets, err := boltio.ListBuckets(path)
			if err != nil {
				return err
			}
			return printNames(cmd, buckets, format)
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
			keys, err := boltio.ListKeys(path, args[0])
			if err != nil {
				return err
			}
			return printNames(cmd, keys, format)
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

			key, err := decodeValue(rawKey, keyFormat)
			if err != nil {
				return fmt.Errorf("decode key: %w", err)
			}

			val, found, err := boltio.Get(path, bucket, string(key))
			if err != nil {
				return err
			}
			if !found {
				return fmt.Errorf("no value at bucket %q key %q", bucket, rawKey)
			}
			s, err := encodeBytes(val, format)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), s)
			return err
		},
	}
	c.Flags().StringVar(&format, "format", "text", formatFlagUsage)
	c.Flags().StringVar(&keyFormat, "key-format", "text", keyFormatFlagUsage)
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

			key, err := decodeValue(rawKey, keyFormat)
			if err != nil {
				return fmt.Errorf("decode key: %w", err)
			}
			value, err := decodeValue(rawValue, format)
			if err != nil {
				return fmt.Errorf("decode value: %w", err)
			}

			opts := wf.writeOptions(cmd)
			opts.Format = format
			_, err = boltio.Put(path, bucket, string(key), value, opts)
			return err
		},
	}
	c.Flags().StringVar(&format, "format", "text", formatFlagUsage)
	c.Flags().StringVar(&keyFormat, "key-format", "text", keyFormatFlagUsage)
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

			var sampleKey string
			if cmd.Flags().Changed("key") {
				decoded, err := decodeValue(key, keyFormat)
				if err != nil {
					return fmt.Errorf("decode key: %w", err)
				}
				sampleKey = string(decoded)
			}

			result, err := boltio.GetSchema(path, bucket, sampleKey)
			if err != nil {
				return err
			}
			return printSchema(cmd, result, format)
		},
	}
	c.Flags().StringVar(&key, "key", "", "sample this specific key instead of the bucket's first key")
	c.Flags().StringVar(&keyFormat, "key-format", "text", keyFormatFlagUsage)
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

			key, err := decodeValue(rawKey, keyFormat)
			if err != nil {
				return fmt.Errorf("decode key: %w", err)
			}

			// Check existence here (as get does) so a missing-key error
			// reports the raw typed key, not the decoded bytes boltio.Patch
			// operates on internally.
			if _, found, err := boltio.Get(path, bucket, string(key)); err != nil {
				return err
			} else if !found {
				return fmt.Errorf("no value at bucket %q key %q", bucket, rawKey)
			}

			opts := wf.writeOptions(cmd)
			// No --format flag: patch always operates on JSON, so the
			// dry-run preview renders as plain text (JSON is text).
			_, err = boltio.Patch(path, bucket, string(key), []byte(fragment), opts)
			return err
		},
	}
	c.Flags().StringVar(&keyFormat, "key-format", "text", keyFormatFlagUsage)
	wf.register(c)
	return c
}
