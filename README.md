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

## Usage

```sh
# Generic
boltdb-cli list-buckets <db-path>
boltdb-cli list-keys <db-path> <bucket>
boltdb-cli get <db-path> <bucket> <key> [--base64]
boltdb-cli put <db-path> <bucket> <key> <value> [--base64] [--dry-run] [--yes|-f]

# Portainer
boltdb-cli portainer get-version <db-path>
boltdb-cli portainer set-version <db-path> [--schema-version X.Y.Z] [--edition N] [--migrator-count N] [--dry-run] [--yes|-f]
boltdb-cli portainer clear-updating-flag <db-path> [--dry-run] [--yes|-f]
```

`--base64` on `get`/`put` encodes/decodes the value as base64, for
binary-safe round-tripping of values that aren't plain text.

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
