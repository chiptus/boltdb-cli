// Package cmd wires the boltdb-cli command tree. Generic bucket/key
// commands sit at the top level; commands that know Portainer's specific
// schema live under the "portainer" subcommand group.
package cmd

import (
	"encoding/base64"
	"fmt"

	"github.com/chiptus/boltdb-cli/internal/boltio"
	"github.com/spf13/cobra"
)

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

func printNames(cmd *cobra.Command, names []string, useBase64 bool) {
	for _, n := range names {
		if useBase64 {
			fmt.Fprintln(cmd.OutOrStdout(), base64.StdEncoding.EncodeToString([]byte(n)))
		} else {
			fmt.Fprintln(cmd.OutOrStdout(), n)
		}
	}
}

func newListBucketsCmd() *cobra.Command {
	var useBase64 bool

	c := &cobra.Command{
		Use:   "list-buckets <db-path>",
		Short: "List every bucket in a bbolt file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			buckets, err := boltio.ListBuckets(args[0])
			if err != nil {
				return err
			}
			printNames(cmd, buckets, useBase64)
			return nil
		},
	}
	c.Flags().BoolVar(&useBase64, "base64", false, "print bucket names as base64 (for non-UTF8 bucket names)")
	return c
}

func newListKeysCmd() *cobra.Command {
	var useBase64 bool

	c := &cobra.Command{
		Use:   "list-keys <db-path> <bucket>",
		Short: "List every key in a bucket",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			keys, err := boltio.ListKeys(args[0], args[1])
			if err != nil {
				return err
			}
			printNames(cmd, keys, useBase64)
			return nil
		},
	}
	c.Flags().BoolVar(&useBase64, "base64", false, "print keys as base64 (many bbolt keys, e.g. Portainer's sequence IDs, are binary, not text)")
	return c
}

func newGetCmd() *cobra.Command {
	var useBase64 bool

	c := &cobra.Command{
		Use:   "get <db-path> <bucket> <key>",
		Short: "Print the value stored at bucket/key",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			val, found, err := boltio.Get(args[0], args[1], args[2])
			if err != nil {
				return err
			}
			if !found {
				return fmt.Errorf("no value at bucket %q key %q", args[1], args[2])
			}
			if useBase64 {
				fmt.Fprintln(cmd.OutOrStdout(), base64.StdEncoding.EncodeToString(val))
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), string(val))
			}
			return nil
		},
	}
	c.Flags().BoolVar(&useBase64, "base64", false, "encode the output as base64 (for binary-safe values)")
	return c
}

func newPutCmd() *cobra.Command {
	var useBase64 bool
	wf := &writeFlags{}

	c := &cobra.Command{
		Use:   "put <db-path> <bucket> <key> <value>",
		Short: "Write a value at bucket/key",
		Args:  cobra.ExactArgs(4),
		RunE: func(cmd *cobra.Command, args []string) error {
			value := []byte(args[3])
			if useBase64 {
				decoded, err := base64.StdEncoding.DecodeString(args[3])
				if err != nil {
					return fmt.Errorf("decode base64 value: %w", err)
				}
				value = decoded
			}

			opts := wf.writeOptions(cmd)
			opts.Base64 = useBase64
			_, err := boltio.Put(args[0], args[1], args[2], value, opts)
			return err
		},
	}
	c.Flags().BoolVar(&useBase64, "base64", false, "decode <value> as base64 (for binary-safe values)")
	wf.register(c)
	return c
}
