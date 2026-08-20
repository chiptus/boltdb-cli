// Package cmd wires the boltdb-cli command tree. Generic bucket/key
// commands sit at the top level; commands that know Portainer's specific
// schema live under the "portainer" subcommand group.
package cmd

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strconv"

	"github.com/chiptus/boltdb-cli/internal/boltio"
	"github.com/spf13/cobra"
)

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

	root.AddCommand(
		newListBucketsCmd(),
		newListKeysCmd(),
		newGetCmd(),
		newPutCmd(),
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
		fmt.Fprintln(cmd.OutOrStdout(), s)
	}
	return nil
}

func newListBucketsCmd() *cobra.Command {
	var format string

	c := &cobra.Command{
		Use:   "list-buckets <db-path>",
		Short: "List every bucket in a bbolt file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			buckets, err := boltio.ListBuckets(args[0])
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
		Use:   "list-keys <db-path> <bucket>",
		Short: "List every key in a bucket",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			keys, err := boltio.ListKeys(args[0], args[1])
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
		Use:   "get <db-path> <bucket> <key>",
		Short: "Print the value stored at bucket/key",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, err := decodeValue(args[2], keyFormat)
			if err != nil {
				return fmt.Errorf("decode key: %w", err)
			}

			val, found, err := boltio.Get(args[0], args[1], string(key))
			if err != nil {
				return err
			}
			if !found {
				return fmt.Errorf("no value at bucket %q key %q", args[1], args[2])
			}
			s, err := encodeBytes(val, format)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), s)
			return nil
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
		Use:   "put <db-path> <bucket> <key> <value>",
		Short: "Write a value at bucket/key",
		Args:  cobra.ExactArgs(4),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, err := decodeValue(args[2], keyFormat)
			if err != nil {
				return fmt.Errorf("decode key: %w", err)
			}
			value, err := decodeValue(args[3], format)
			if err != nil {
				return fmt.Errorf("decode value: %w", err)
			}

			opts := wf.writeOptions(cmd)
			opts.Format = format
			_, err = boltio.Put(args[0], args[1], string(key), value, opts)
			return err
		},
	}
	c.Flags().StringVar(&format, "format", "text", formatFlagUsage)
	c.Flags().StringVar(&keyFormat, "key-format", "text", keyFormatFlagUsage)
	wf.register(c)
	return c
}
