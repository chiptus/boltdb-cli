Status: ready-for-agent

# Generic `patch` command (JSON merge-patch)

Superseded by `.scratch/boltdb-cli/patch-spec.md` — grilling resolved every
open question below; that spec is the implementation-ready document.

## Problem

The v1 spec (`.scratch/boltdb-cli/spec.md`) deliberately left out a generic
merge-patch command — `portainer set-version` hand-rolls its own
read-modify-write logic (decode JSON, mutate only the fields the caller
passed, re-encode, write through `boltio.Put`). That logic only exists for
Portainer's `Version` struct. Any other JSON value stored in a bbolt file
(Portainer's or otherwise) has no equivalent partial-update path — the only
option is `get`, hand-edit the full JSON text, then `put` the whole thing
back, which risks clobbering fields the caller didn't mean to touch.

## Proposed shape

`boltdb-cli patch <bucket> <key> <json-fragment>`

- Reads the current value at bucket/key, JSON-decodes it into
  `map[string]any`.
- JSON-decodes `<json-fragment>` the same way.
- Applies an RFC 7396 JSON merge-patch: keys in the fragment overwrite keys
  in the current value; a fragment value of `null` deletes that key.
- Re-encodes and writes through `boltio.Put`, inheriting the existing
  backup/dry-run/confirm safety net (same `writeFlags` as `put`).

## Open questions (candidates for grilling)

- What happens when the current value isn't valid JSON, or isn't a JSON
  object (e.g. a JSON array, or a plain scalar/uint64-be value)? Error
  clearly rather than silently coercing.
- Does this replace or coexist with `portainer set-version`'s hand-rolled
  merge logic? (Spec's stance was that `set-version` is "Portainer-schema-
  aware logic, not something the generic layer needs to understand" — worth
  re-litigating now that a generic version would exist.)
- Nested-object merge semantics: RFC 7396 merges recursively for nested
  objects — confirm that's the desired behavior vs. a shallow top-level-keys-
  only merge.
- Key/value encoding: does `patch` need `--key-format` like `get`/`put`, even
  though the value itself must be JSON?

## Relates to

- `.scratch/boltdb-cli/issues/02-get-schema-command.md` — a `get-schema`
  command would help a caller know what fields exist before patching them.
