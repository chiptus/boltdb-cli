Status: ready-for-agent

# boltdb-cli

## Problem Statement

Portainer stores its schema version (and a mid-migration "updating" flag) inside the boltdb data file itself. When a local Portainer data directory is reused with a binary of a different version, the stored version can end up ahead of what the binary expects, and Portainer won't migrate or boot cleanly. Separately, if Portainer crashes mid-migration, the stored "updating" flag can get stuck on, which also blocks boot. Today the only fix is opening the `.db` file in `boltbrowser` and hand-editing raw bucket/key values as text — a generic tool with no awareness of what it's editing, used to patch a very specific, error-prone value by hand, with no backup safety net.

There's no scriptable, safe way to inspect or patch values inside a bbolt file, whether for Portainer's version bucket specifically or for bbolt files generally.

## Solution

A standalone CLI, `boltdb-cli`, that provides:

- Generic, scriptable primitives for reading and writing any bucket/key in any bbolt file (a safer, scriptable replacement for the relevant slice of `boltbrowser`'s functionality).
- A thin `portainer` subcommand group built on top of those primitives that knows Portainer's specific version-bucket schema, so a user can directly `get`/`set` the stored schema version or clear a stuck "updating" flag without hand-editing JSON text.
- Non-negotiable safety: every write is preceded by a timestamped backup of the original file, supports a `--dry-run` preview, and prompts for interactive confirmation unless explicitly skipped.

## User Stories

1. As a Portainer developer, I want to view the current stored schema version of a local Portainer data file, so that I can understand why a given binary is refusing to boot against it.
2. As a Portainer developer, I want to set the stored schema version on a local Portainer data file, so that I can boot an older binary against a data directory that was previously opened by a newer one.
3. As a Portainer developer, I want to set the stored edition value on a local Portainer data file, so that I can correct an edition mismatch between the binary and the stored data.
4. As a Portainer developer, I want to set the stored migrator count on a local Portainer data file, so that I can align it with the schema version I'm setting.
5. As a Portainer developer, I want to clear a stuck "DB is updating" flag, so that I can boot Portainer after a crash mid-migration without it refusing to start.
6. As a user of the CLI, I want every write operation to automatically back up the original file first, so that I never lose data to a bad edit.
7. As a user of the CLI, I want a `--dry-run` flag on write operations, so that I can preview what would change before committing to it.
8. As a user of the CLI, I want an interactive confirmation prompt before any write takes effect, so that I don't accidentally overwrite something by a typo or wrong argument.
9. As a user of the CLI, I want to skip the confirmation prompt with a `--yes`/`-f` flag, so that I can script the tool in automated workflows.
10. As a user of any bbolt file (not just Portainer's), I want to list all buckets in a `.db` file, so that I can explore its structure without a GUI tool.
11. As a user of any bbolt file, I want to list all keys within a given bucket, so that I can find the key I need to inspect or change.
12. As a user of any bbolt file, I want to get the raw value stored at a given bucket/key, so that I can inspect it as text.
13. As a user of any bbolt file, I want to put a raw value at a given bucket/key, so that I can make a direct edit without a full GUI browser.
14. As a user of any bbolt file, I want to get/put binary-safe values via a `--base64` flag, so that non-UTF8 values round-trip correctly instead of getting corrupted by shell string handling.
15. As a maintainer of this tool, I want the version-set logic to validate that the provided schema version string is valid semver, so that obvious typos are caught before being written.
16. As a maintainer of this tool, I want the version-set logic to NOT validate against a whitelist of real Portainer releases, so that the tool doesn't need to be updated every time Portainer ships a new version.
17. As a reader of the README, I want an explicit callout that edition mismatches between EE and CE are not validated by this tool, so that I understand the tool's limits before trusting its output on an EE database.
18. As a public GitHub visitor, I want the repo to carry a permissive open-source license, so that I know I'm free to use and modify the tool.
19. As a contributor, I want CI to run build/vet/test on every push and PR, so that regressions are caught automatically.
20. As a Portainer developer setting `--schema-version`, I want the other fields of the stored version struct (edition, migrator count, instance ID) preserved unless I explicitly change them, so that a targeted schema-version fix doesn't silently wipe unrelated stored state.

## Implementation Decisions

- **Module/repo naming**: the repo and Go module are named `boltdb-cli` (not `portainer-db-cli` as originally proposed), module path `github.com/chiptus/boltdb-cli`. The tool's purpose (fixing Portainer version mismatches) is served by a `portainer` subcommand group, not by the top-level tool identity — because the core is a generic bbolt editor, not a Portainer-specific one.
- **No dependency on `portainer-suite`**: the Portainer `Version` struct (`SchemaVersion string`, `MigratorCount int`, `Edition int`, `InstanceID string`) and the bucket/key names (`"version"` bucket, `"VERSION"` and `"DB_UPDATING"` keys) are hand-written in this repo, not vendored or imported from `portainer/portainer-suite`. Values are marshaled with stdlib `encoding/json`, which is byte-compatible with the `segmentio/encoding/json` codec Portainer itself uses for this struct shape. This is deliberate: `boltbrowser`, the existing manual workaround, already treats these values as opaque JSON text with zero schema knowledge, which proves no dependency on Portainer's own code is needed to read/write this value correctly.
- **Architecture — generic core, thin Portainer layer**: a generic `boltio`-style core provides bucket/key primitives (list buckets, list keys, get, put) operating on raw values (string, or base64-encoded bytes via a `--base64` flag). All backup-before-write, dry-run, and confirmation-prompt logic lives in this core write path, so every write anywhere in the tool inherits it. The `portainer` subcommand group is a thin wrapper: `get-version` reads and JSON-decodes the version key; `set-version` reads the current struct, mutates only the fields the user passed flags for, re-encodes, and writes through the same generic write path; `clear-updating-flag` writes `false` to the `DB_UPDATING` key through the same path.
- **No generic merge-patch command in v1**: the handoff's "Enhancement 1" (a generic RFC-6902/merge-patch style `patch` command) is deliberately out of scope. The read-modify-write behavior needed for `set-version` is implemented directly inside the `portainer` wrapper, not as a call to a generic patch primitive — it's Portainer-schema-aware logic, not something the generic layer needs to understand.
- **Command grouping**: generic commands are flat at the top level (`boltdb-cli list-buckets`, `list-keys`, `get`, `put`). Portainer-specific commands live under a `portainer` subcommand group (`boltdb-cli portainer get-version`, `portainer set-version`, `portainer clear-updating-flag`), so the boundary between "knows nothing about Portainer" and "Portainer-schema-aware" is visible in the command tree itself.
- **Safety semantics on every write** (`put`, `set-version`, `clear-updating-flag`):
  1. Copy the original `.db` file to `<name>.bak-<timestamp>` before any modification, unconditionally — no flag disables this.
  2. If `--dry-run` is passed, print what would change and exit without writing or backing up.
  3. Otherwise, prompt for interactive confirmation before writing, unless `--yes`/`-f` is passed.
- **Value encoding**: `get`/`put`/`dump` operate on string values by default (sufficient for Portainer's JSON values and most human-readable bbolt content). A `--base64` flag on both read and write paths switches to base64-encoded input/output for binary-safe round-tripping of arbitrary byte values.
- **Semver validation**: `portainer set-version --schema-version` validates the string via `golang.org/x/mod/semver` (`semver.IsValid`) only. It does not check the value against a list of real Portainer releases — the caller is trusted, since the backup/dry-run/confirmation safety net already covers the "oops, bad value" case, and a hardcoded release whitelist would need permanent maintenance as Portainer ships new versions.
- **Edition mismatch is undocumented in code, documented in prose**: the tool does not check whether a target binary is CE or EE, or whether an edition value is compatible with it (this was an open question in the original handoff and was deliberately not resolved by reading `portainer-suite`'s EE migrator source). The README must explicitly state that edition mismatches are the caller's responsibility.
- **CLI framework**: `spf13/cobra`, given the nested subcommand structure (generic commands + a `portainer` group).
- **License**: MIT, applied at the repo root. No Portainer source is vendored, so there's no obligation to carry Portainer's zlib license.
- **Repo bootstrapping**: built directly in the already-created local directory (no GitHub remote created yet); `git init` and any push to `github.com/chiptus/boltdb-cli` happen as a separate, later step, not part of this spec's implementation.

## Testing Decisions

- **Test seam**: tests invoke each cobra command's `RunE` directly with arguments, against a real temporary bbolt file created per test (no mocking of bbolt) — then assert on the resulting file state by reading it back directly with `bbolt`, and by checking backup-file existence/contents for write paths. This is the single seam for the whole tool: both the generic primitives and the `portainer` wrapper commands are exercised through the same command-invocation path, since the wrapper commands are themselves built only from the generic primitives.
- **What makes a good test here**: assert observable outcomes only — the bytes actually stored at a bucket/key after a command runs, the existence and contents of a backup file, the exit code/output of a `--dry-run` invocation (nothing written, backup not created), and the printed output of `get`/`get-version`. Do not assert on internal call sequences or mock the confirmation prompt's implementation — feed it via stdin or bypass it with `--yes` in tests that aren't specifically testing the prompt itself.
- **Modules under test**: the generic primitives (`list-buckets`, `list-keys`, `get`, `put`, including `--base64` and the shared backup/dry-run/confirm write path) and the `portainer` wrapper commands (`get-version`, `set-version` including partial-field mutation, `clear-updating-flag`).
- **Prior art**: none in this repo yet (greenfield). No comparable prior art exists in `portainer-suite` either — `helper-reset-password` is a precedent for standalone-tool structure, not for its test approach.
- **Process**: TDD — tests for each command are written before its implementation.

## Out of Scope

- Vendoring or depending on `github.com/portainer/portainer` or the private `portainer-suite` monorepo.
- A generic RFC-6902/merge-patch `patch` command (handoff's "Enhancement 1").
- Validating `--schema-version` against a whitelist of real Portainer releases.
- Any code-level check of CE/EE edition compatibility (`server-ee` migrator behavior was never confirmed).
- Creating or pushing to the `github.com/chiptus/boltdb-cli` GitHub repository — this spec covers local implementation only.
- A fully schema-agnostic interactive TUI browser (handoff's "Enhancement 2" beyond the flat `list-buckets`/`list-keys`/`get`/`put` primitives already included here — no interactive/curses-style UI is in scope, only scriptable CLI commands).

## Further Notes

- This spec supersedes the original `portainer-db-cli` handoff doc in naming and architecture: the generic bbolt primitives are now the foundation the Portainer commands are built on, rather than a stretch enhancement bolted onto a Portainer-first tool.
- `boltbrowser`'s existing behavior (treating stored values as opaque text, with zero Portainer-specific code) is the reason vendoring Portainer's struct/codec was judged unnecessary — it's direct evidence that schema knowledge isn't required to safely read/write this value.
- CI (GitHub Actions running `go build`, `go vet`, `go test ./...` on push/PR) should be scaffolded as part of this implementation, even though the repo has no remote yet.
