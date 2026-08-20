Status: ready-for-agent

# `get-schema` command (agent-facing shape introspection)

## Problem Statement

bbolt buckets are schemaless — a bucket is just an opaque list of key/value byte pairs. An agent or human inspecting an unfamiliar `.db` file has to run `get` on individual keys and eyeball the raw JSON to figure out what fields exist before it can safely `put`/`patch` anything. There's no single command that answers "what does a value in this bucket look like?"

## Solution

A new top-level command, `boltdb-cli get-schema <bucket>`, that samples one value from the bucket, JSON-decodes it, and prints the inferred field names and types — explicitly labeled as inferred from a sample, not a real schema, since other keys in the same bucket could have a different shape. Ambiguous fields (`null`, empty arrays) trigger a narrow, bounded fallback scan of a few more keys to try to resolve a concrete type. Output is available both as a human-readable tree and as `--format json` for agent consumption.

## User Stories

1. As an agent inspecting an unfamiliar bbolt file, I want to see the field names and types of values in a bucket, so that I can decide what to `get`/`put`/`patch` without first hand-decoding raw JSON.
2. As a human exploring a `.db` file, I want a human-readable tree of a bucket's inferred shape, so that I can understand its structure at a glance.
3. As an agent scripting against this tool, I want a `--format json` output for the inferred shape, so that I can parse it reliably instead of scraping indented text.
4. As a user, I want `get-schema` to default to sampling the bucket's first key, so that I don't have to know a specific key name just to see the shape.
5. As a user, I want a `--key` flag to pick a specific key to sample, so that I can inspect a value I already know is representative (or suspect is an outlier).
6. As a user with non-text keys (e.g. Portainer's `NextSequence()`-keyed buckets), I want a `--key-format` flag matching `get`/`put`/`patch`, so that I can target a specific key by its typed value (e.g. `uint64-be`).
7. As a user, I want a clear, non-error message when the sampled value isn't JSON at all (plain text, `uint64-be`, binary), so that I get a useful answer instead of a crash or garbled output.
8. As a user, I want a non-object top-level JSON value (a bare array or scalar) reported with its own inferred type, so that `get-schema` doesn't require every bucket to store JSON objects.
9. As a user, I want nested JSON objects to be recursed into and reported as a nested shape, so that I can see the full structure of complex values, not just the top level.
10. As a user, I want array fields typed from their first element (e.g. `array<string>`, or `array<empty>` for a zero-length array), so that I get a useful type without the tool attempting full mixed-type detection.
11. As a user, I want every inferred-type line to state clearly that it's inferred from a sample (e.g. "inferred from key \"...\", 1 sample"), so that I don't mistake the output for a guaranteed schema.
12. As a user, I want fields whose primary-sample value is ambiguous (`null`, empty array) to be resolved against a few other keys in the bucket when possible, so that a single unlucky sample doesn't make the whole field report useless as `null`.
13. As a user, I want the fallback scan to be bounded (not scan the whole bucket) and to stop as soon as it finds a resolving value, so that `get-schema` stays fast on large buckets.
14. As a user, I want a field resolved via the fallback scan to be labeled distinctly from a field resolved in the primary sample (e.g. "resolved from key \"...\", ambiguous in sample"), so that I know which fields came from the primary sample and which needed a secondary lookup.
15. As a maintainer, I want `get-schema` to sit at the top level alongside `list-buckets`/`list-keys`/`get`/`put`/`patch`, not under the `portainer` subcommand group, so that the command tree keeps reflecting the "generic first" architecture already established by `patch`.

## Implementation Decisions

- **Module**: a new `boltio.GetSchema(path, bucket string, opts ...)` function in `internal/boltio`, following the existing package's pattern (see `ListKeys`, `Get`, `Patch`). It returns an inferred-schema result structure (field name → inferred type, recursively for nested objects), not a pre-rendered string, so the `cmd` layer can render it as either a tree or JSON.
- **Command**: a new `newGetSchemaCmd()` in `cmd/root.go`, registered in `NewRootCmd()` alongside the other top-level commands. `Use: "get-schema <bucket>"`, `Args: cobra.ExactArgs(1)`.
- **Flags**: `--key` (string, optional; defaults to the bucket's first key via `ListKeys`), `--key-format` (matching `keyFormatFlagUsage`, same as `get`/`put`/`patch`), `--format` (`text` default / `json`, matching the existing `--format` convention on `list-buckets`/`list-keys`/`get`). No write-related flags — this is a read-only command with no `writeFlags`.
- **Primary sample resolution**: resolve the sample key exactly as `get` does — via `--key`/`--key-format` if provided, else the first key from `ListKeys`. Fetch its value via the existing `boltio.Get`.
- **Non-JSON handling**: if the primary sample's value fails to JSON-decode, report it as not-JSON (e.g. `(not JSON — <n> bytes, raw text/binary)`) and exit 0 — this is not an error condition.
- **Non-object top-level values**: if the decoded JSON is a bare array or scalar (not an object), report its inferred type directly (e.g. `array<number>`, `number`, `string`) rather than requiring object shape.
- **Type inference**: for each field in an object (recursing into nested objects), infer one of `string`, `number`, `bool`, `null`, `array<T>` (typed from the first element only; `array<empty>` for a zero-length array), or a nested object shape.
- **Ambiguous fields and fallback scan**: a field is ambiguous if its primary-sample type is `null` or `array<empty>`. For each ambiguous field, scan additional keys in the bucket (via `ListKeys`, skipping the primary sample key), decoding each and checking whether that specific field resolves to a non-ambiguous type, stopping at the first resolution. Cap the scan at a small fixed number of additional keys checked (e.g. 20) — if the cap is reached without resolution, report the field as its original ambiguous type. This fallback is narrow: it only targets specific ambiguous fields, not general shape-drift detection across the whole bucket's fields.
- **Output labeling**: every inferred-type line carries a caveat that it's inferred from a sample (e.g. "(inferred from key \"...\", 1 sample)" at the top of the tree). A field resolved via the fallback scan is labeled distinctly from one resolved in the primary sample (e.g. "(resolved from key \"...\", ambiguous in sample)"), so the two provenances are visually distinguishable.
- **Output rendering**: default output is a human-readable indented tree (field name, inferred type, nested indentation for objects). `--format json` emits a machine-parseable shape description, e.g. `{"field": "string", "nested": {"field2": "number"}}`, with array types rendered as `"array<string>"` etc. and ambiguous/resolved provenance omitted from the JSON form or carried as a parallel metadata field (implementer's call, consistent with keeping the JSON output easy to consume programmatically).
- **Command placement**: top level (`boltdb-cli get-schema <bucket>`), not under the `portainer` subcommand group — following the precedent set by `patch`, which is also JSON-aware but lives at the top level rather than being scoped to Portainer's schema.

## Testing Decisions

- **What makes a good test here**: assert on the returned inferred-schema structure / rendered output for `boltio.GetSchema`, and on the printed CLI output for the `get-schema` command — not on internal call sequences. This is a read-only command, so there's no backup/dry-run/confirm behavior to test (unlike `put`/`patch`).
- **Seam**: mirrors the existing `Patch`/`patch` pairing exactly — `boltio.GetSchema` tested directly against a real temporary bbolt file in `internal/boltio/boltio_test.go` (no mocking of bbolt), and the `get-schema` cobra command tested by invoking its `RunE` directly in `cmd/root_test.go`. No new seams introduced.
- **Modules under test**:
  - `internal/boltio`: `GetSchema` against object-shaped values, nested objects, non-object top-level values (array, scalar), non-JSON values, arrays (including empty), `null` fields with and without a resolving fallback key, and the fallback-scan cap being reached without resolution.
  - `cmd`: `get-schema` with default key selection, `--key`/`--key-format`, `--format json` vs default tree, and the not-JSON / non-object-top-level messages surfacing correctly through the CLI.
- **Prior art**: `TestGet`/`TestPatch*` in `boltio_test.go` and `TestGetCmd*`/`TestPatchCmd*` in `root_test.go` are the direct precedent for both layers.

## Out of Scope

- General shape-drift detection across a bucket's full field set (only the narrow ambiguous-field fallback scan is in scope).
- Mixed-element-type detection within arrays (only the first element's type is used).
- Any write behavior — `get-schema` is read-only, with no backup/dry-run/confirm flags.
- Validating or enforcing a schema — this command only infers and reports, never rejects a value for not matching an expected shape.

## Further Notes

- This spec resolves all open questions from `.scratch/boltdb-cli/issues/02-get-schema-command.md` via a grilling/domain-modeling session on 2026-08-20; the resulting vocabulary (Inferred schema, Primary sample, Ambiguous field, Fallback scan) now lives in the repo's `CONTEXT.md`.
- Relates to `.scratch/boltdb-cli/patch-spec.md` — knowing a bucket's inferred shape is a natural precursor to patching specific fields in it.
