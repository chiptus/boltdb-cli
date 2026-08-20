Status: ready-for-agent

# boltdb-cli: generic `patch` command

This spec is a follow-on to `.scratch/boltdb-cli/spec.md` (the v1 tool spec),
which shipped `list-buckets`, `list-keys`, `get`, and `put` on top of the
shared `boltio` backup/dry-run/confirm write path, and deliberately left a
generic JSON merge-patch command out of scope. It assumes that v1
architecture (generic `boltio` core, `writeFlags`, `resolveDBPath`,
`encodeBytes`/`decodeValue`) as given and does not restate it.

## Problem Statement

Some values stored in a bbolt bucket are JSON objects (Portainer's `version`
key being one example, but not the only one). Today the only way to change
one field of such a value is `get` the whole thing, hand-edit the full JSON
text, and `put` the entire value back — which risks silently clobbering
fields the caller never meant to touch, and is exactly the kind of
error-prone manual editing this tool exists to replace. `portainer
set-version` solves this for one specific struct by hand-rolling its own
read-decode-mutate-encode-write logic, but that logic is Portainer-specific
and doesn't help with any other JSON value in any other bbolt file.

## Solution

A generic `patch <bucket> <key> <json-fragment>` command that reads the
current JSON object stored at bucket/key, applies `<json-fragment>` to it as
an RFC 7396 JSON Merge Patch, and writes the result back through the
existing `boltio.Put` safety net (backup, `--dry-run`, confirmation prompt).
This lets a caller change just the fields they name, on any JSON-object
value in any bbolt file, without hand-editing the rest of the value or
writing bespoke Go code for each struct shape.

## User Stories

1. As a user of any bbolt file, I want to patch a single field of a JSON
   object value without retyping the whole object, so that I don't
   accidentally drop or corrupt fields I didn't mean to change.
2. As a user of any bbolt file, I want to patch multiple fields, including
   nested objects, in one `patch` invocation, so that a multi-field fix
   doesn't require several round-trips.
3. As a user of any bbolt file, I want to delete a field from a stored JSON
   object by patching it with a `null` value, so that I can remove
   obsolete/incorrect keys without a full rewrite.
4. As a user of any bbolt file, I want `patch` to inherit `--dry-run`, so
   that I can preview the merged result before committing to it.
5. As a user of any bbolt file, I want `patch` to inherit the interactive
   confirmation prompt and `--yes`/`-f` bypass, so that its safety behavior
   is identical to every other write command in this tool.
6. As a user of any bbolt file, I want every `patch` write to be preceded by
   the same timestamped backup as `put`, so that a bad patch is always
   recoverable.
7. As a user of any bbolt file with binary (non-UTF8) keys, I want `patch`
   to support `--key-format` the same way `get`/`put` do, so that I'm not
   blocked from patching values in such buckets.
8. As a user of any bbolt file, I want a clear, specific error when the
   bucket/key doesn't exist yet, so that I know `patch` doesn't silently
   create new keys from just a fragment.
9. As a user of any bbolt file, I want a clear, specific error when the
   stored value isn't valid JSON at all, so that I understand why `patch`
   refused to run rather than seeing a confusing decode panic or garbled
   output.
10. As a user of any bbolt file, I want a clear, specific error when the
    stored value is valid JSON but not a JSON object (e.g. an array or a
    bare number/string), so that I understand merge-patch semantics don't
    apply to it.
11. As a user of any bbolt file, I want a clear, specific error when
    `<json-fragment>` itself isn't valid JSON, so that a shell-quoting typo
    is caught with a useful message instead of a stack trace.
12. As a user of any bbolt file, I want a clear, specific error when
    `<json-fragment>` is valid JSON but not a JSON object, so that I
    understand why a bare scalar or array can't be merge-patched in.
13. As a Portainer developer, I want `portainer set-version`'s existing
    semver-validated, field-by-field behavior to keep working exactly as it
    does today, so that adding a generic `patch` command doesn't change or
    risk the tool's original, most load-bearing use case.
14. As a maintainer of this tool, I want the merge-patch logic implemented
    via a well-established library rather than hand-rolled, so that RFC 7396
    edge cases (recursive nested merges, null-deletes-key) are handled
    correctly without this repo needing to re-derive and re-test the spec
    itself.

## Implementation Decisions

- **Command shape**: `patch <bucket> <key> <json-fragment>`, a new
  top-level generic command alongside `list-buckets`/`list-keys`/`get`/`put`
  (flat command tree, per the v1 spec's command-grouping decision — this is
  schema-agnostic, so it does not belong under the `portainer` subcommand
  group).
- **Fragment input**: positional argument only, matching `put`'s
  `<value>` convention. No `--file`/stdin input in this iteration.
- **Merge library**: `github.com/evanphx/json-patch/v5`
  (`jsonpatch.MergePatch`), added as a new module dependency. This
  implements RFC 7396 (recursive nested merge, a `null` value in the
  fragment deletes the corresponding key) — the standard "JSON Merge Patch"
  semantics, not RFC 6902 "JSON Patch" (a different, operation-list format
  that the same library also happens to implement but which is not used
  here). Rationale for a library over hand-rolling: this is the de facto
  standard Go implementation of RFC 7396 (used by Kubernetes, Docker),
  actively maintained, permissively licensed (BSD-3-Clause), and exposes
  exactly the needed two-function API — using it avoids this repo having to
  re-derive and re-test RFC 7396's edge cases itself.
- **Validation ordering and errors**: before attempting any merge, `patch`
  validates, and errors distinctly on, each of:
  1. bucket/key absent — same "no value at bucket %q key %q" style error
     `get` already produces; `patch` never creates a new key from a
     fragment alone.
  2. stored value is not valid JSON — a decode error distinct from case 1.
  3. stored value is valid JSON but not a JSON object (e.g. array, string,
     number) — a distinct "not a JSON object" error; merge-patch is only
     defined for object-to-object merges.
  4. `<json-fragment>` argument is not valid JSON — a decode error, checked
     independently of the stored value's validity.
  5. `<json-fragment>` is valid JSON but not a JSON object — same "not a
     JSON object" treatment as case 3, applied to the fragment.
- **Write path reuse**: `patch` reads the current value via the existing
  `boltio.Get`, computes the merged JSON via `jsonpatch.MergePatch`, and
  writes the result via the existing `boltio.Put` — inheriting backup,
  `--dry-run`, and confirmation-prompt behavior unchanged. No new safety
  logic is introduced.
- **Dry-run preview**: reuses `put`'s existing old/new rendering exactly —
  full old and new JSON text is shown, not a field-level diff. `patch`'s
  `WriteOptions.Format` is effectively always `"text"` (JSON is textual), so
  no format-rendering changes are needed in `boltio.Put`.
- **Flags**: `--key-format` (text/base64/hex/uint64-be) is added, identical
  in behavior to `get`/`put`'s flag of the same name, for buckets with
  binary keys. `--format` is deliberately *not* added to `patch` — unlike
  `get`/`put`, `patch`'s value is always a JSON object, so base64/hex/
  uint64-be rendering options would be meaningless dead flags. `patch`
  reuses `writeFlags` (`--dry-run`, `--yes`/`-f`) exactly as `put` does.
- **No change to `portainer.SetVersion`**: it keeps its own hand-rolled
  read-mutate-write logic (including `--schema-version` semver validation),
  unchanged and not refactored to call the generic `patch`/merge logic
  internally. The v1 spec's boundary — `set-version` is "Portainer-schema-
  aware logic, not something the generic layer needs to understand" — is
  reaffirmed rather than revisited. Any future refactor to share code
  between them is out of scope here.

## Testing Decisions

- **Seam**: identical to the seam already established in `spec.md` for
  every other command — invoke the new `patch` command's `RunE` directly
  against a real temporary bbolt file (no bbolt mocking), then assert on
  observable outcomes: the resulting bytes at bucket/key read back directly
  via `bbolt`, the existence/contents of the timestamped backup file for
  writes, and the printed dry-run/confirmation output. This is the same
  single seam used for `get`/`put`'s tests; no new test infrastructure is
  introduced.
- **What to test**: a successful single-field merge; a successful
  multi-field and nested-object merge; field deletion via a `null` fragment
  value; each of the five validation-error cases (absent key, non-JSON
  stored value, non-object stored value, non-JSON fragment, non-object
  fragment) produces its own distinct error and does not write or back up
  anything; `--dry-run` previews the merged result without writing or
  creating a backup; `--yes` bypasses the confirmation prompt; `--key-format`
  correctly decodes a non-text key before doing the lookup/write.
- **Prior art**: this repo's existing `get`/`put` command tests are the
  direct precedent for both the seam and the assertion style.

## Out of Scope

- `--file`/stdin input for the JSON fragment (positional arg only, for now).
- A field-level diff view in `--dry-run` output (full old/new JSON only).
- `--format` on `patch` (value is always JSON; N/A).
- Refactoring `portainer.SetVersion` to share code with the generic `patch`
  logic.
- Auto-creating a new bucket/key from a fragment when the target doesn't
  exist yet.
- The separate `get-schema` command proposed alongside this one (tracked
  independently in
  `.scratch/boltdb-cli/issues/02-get-schema-command.md`) — related, but not
  part of this spec.
- Multi-sample or shape-drift detection of any kind (not applicable to
  `patch`, which only ever touches the one bucket/key it's given).

## Further Notes

- This spec supersedes
  `.scratch/boltdb-cli/issues/01-patch-command.md`'s open questions — all of
  them were resolved during grilling and are captured as Implementation
  Decisions above. That issue file's "Open questions" section can be
  considered closed by this document.
- `.scratch/boltdb-cli/issues/02-get-schema-command.md` remains a separate,
  not-yet-grilled ticket; it is related (schema introspection is a natural
  precursor to patching) but intentionally out of scope here.
