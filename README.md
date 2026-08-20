# boltdb-cli

A scriptable, safe alternative to hand-editing [bbolt](https://github.com/etcd-io/bbolt)
database files in a GUI browser like `boltbrowser`.

It provides two layers:

- **Generic commands** (`list-buckets`, `list-keys`, `get`, `put`) that work
  on any bbolt file, with no schema knowledge.
- **`portainer` commands** (`get-version`, `set-version`,
  `clear-updating-flag`) — a thin, Portainer-schema-aware layer built on top
  of the generic commands, for fixing the "DB is a newer version, won't
  boot" problem when bouncing a local Portainer data directory between
  binary versions.

## Safety

Every write (`put`, `portainer set-version`, `portainer clear-updating-flag`)
is:

1. Backed up first — the original file is copied to `<path>.bak-<timestamp>`
   before any write, unconditionally.
2. Previewable with `--dry-run`, which prints the old/new values and exits
   without writing or backing up.
3. Confirmed interactively before writing, unless `--yes`/`-f` is passed.

## Database path

Every command needs a database path. It's never a positional argument —
give it either way:

- `--db <path>`, a persistent flag accepted by every command, or
- the `BOLTDB_CLI_PATH` environment variable, so you can set it once per
  shell session and drop it from every subsequent invocation:

  ```sh
  export BOLTDB_CLI_PATH=~/portainer-data/ee/portainer.db
  boltdb-cli list-buckets
  boltdb-cli portainer get-version
  ```

`--db` takes precedence over `BOLTDB_CLI_PATH` when both are set.

## Usage

```sh
# Generic
boltdb-cli list-buckets [--format text|base64|hex|uint64-be]
boltdb-cli list-keys <bucket> [--format text|base64|hex|uint64-be]
boltdb-cli get <bucket> <key> [--key-format ...] [--format ...]
boltdb-cli put <bucket> <key> <value> [--key-format ...] [--format ...] [--dry-run] [--yes|-f]

# Portainer
boltdb-cli portainer get-version
boltdb-cli portainer set-version [--schema-version X.Y.Z] [--edition N] [--migrator-count N] [--dry-run] [--yes|-f]
boltdb-cli portainer clear-updating-flag [--dry-run] [--yes|-f]
```

Both flags accept `text` (default), `base64`, `hex`, or `uint64-be`.

- On `list-buckets`/`list-keys`, `--format` controls how the listed
  bucket/key names are printed.
- On `get`/`put`, `--key-format` controls how the `<key>` argument is
  decoded (it's always plain text on the command line, decoded to raw bytes
  before the lookup/write), and `--format` controls how the value is
  encoded (`get`, and previews) or decoded (`put`) — independently, since a
  bucket's keys and values are often encoded differently.

Use `base64`/`hex` for binary-safe round-tripping of values that aren't
plain text. Use `uint64-be` for keys that are bbolt `NextSequence()`-derived
8-byte big-endian integers — this is how Portainer keys most entity
buckets (`teams`, `users`, `endpoints`, ...) — to read/write them as plain
decimal IDs instead of base64/hex, e.g.:

```sh
boltdb-cli list-keys users --format uint64-be
boltdb-cli get users 1 --key-format uint64-be
```

## Portainer version semantics

`portainer set-version` patches only the fields you pass — any field you
don't pass (including `InstanceID`, which has no flag) is preserved as-is.
`--schema-version` is validated as semver only; it is **not** checked
against a list of real Portainer releases, so a typo like `99.99.99` will
be accepted. Rely on the backup/dry-run/confirmation safety net instead.

**Edition mismatches are not validated.** This tool does not check whether
`--edition` is compatible with the Portainer binary (CE vs. EE) that will
open the database. Setting an incompatible edition value is the caller's
responsibility.

## License

MIT — see [LICENSE](./LICENSE).
