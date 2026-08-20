Status: ready-for-agent

Superseded by `.scratch/boltdb-cli/get-schema-spec.md` — grilling resolved
every open question below; that spec is the implementation-ready document.

# `get-schema <bucket>` command (agent-facing shape introspection)

## Problem

bbolt has no schema — a bucket is just an opaque list of key/value byte
pairs. An agent (or a human) inspecting an unfamiliar `.db` file has to run
`get` on individual keys and eyeball the raw JSON to figure out what fields
exist before it can safely `put`/`patch` anything. There's no single command
that answers "what does a value in this bucket look like?"

## Proposed shape

`boltdb-cli get-schema <bucket>` — top-level generic command (same
placement precedent as `patch`).

- Picks a **primary sample** key from the bucket: first key via
  `ListKeys`, or a specific one via `--key` (with a `--key-format` flag,
  matching `get`/`put`/`patch`).
- JSON-decodes the primary sample. If it isn't valid JSON (plain text,
  `uint64-be`, binary), report e.g. `(not JSON — <n> bytes, raw
  text/binary)` and exit 0 — not an error.
- If the top-level value isn't a JSON object (a bare array or scalar),
  report its inferred type directly (e.g. `array<number>`, `number`)
  instead of requiring object shape.
- Prints field names and inferred types (string/number/bool/`null`/
  `array<T>`/nested object), recursing into nested objects. Arrays are
  typed from their first element only (`array<T>`, or `array<empty>` for
  a zero-length array) — no mixed-element-type detection in v1.
- **Ambiguous fields** (`null`, `array<empty>`) trigger a bounded
  **fallback scan**: check additional keys in the bucket (capped, e.g. 20)
  for one where that field resolves to a concrete type, stopping at the
  first resolution. This is narrow — it only resolves specific ambiguous
  fields, not general shape-drift detection across the bucket. A
  fallback-resolved field is labeled distinctly, e.g. `(resolved from key
  "...", ambiguous in sample)`.
- Output explicitly labeled as inferred from a sample (e.g. `(inferred from
  key "...", 1 sample)`), since it is not a real schema — other keys in the
  same bucket could have different shapes.
- Default output is a human-readable tree; `--format json` also emits a
  machine-parseable shape description (e.g. `{"field": "string", "nested":
  {"field2": "number"}}`), matching the `--format` convention on
  `list-buckets`/`list-keys`/`get`, since this command is explicitly
  agent-facing.

## Resolved (grilling session, 2026-08-20)

All open questions below were resolved during a grilling/domain-modeling
session — see the terms now in `CONTEXT.md` (Inferred schema, Primary
sample, Ambiguous field, Fallback scan).

- Non-JSON primary sample: reported gently, not an error.
- Sampling stays single-sample for v1 (no general shape-drift detection),
  except for the narrow ambiguous-field fallback scan above.
- `--key`/`--key-format` included for explicit key selection.
- Top-level placement, following `patch`'s precedent.
- Both human-readable tree and `--format json` output.
- Arrays typed from first element (`array<T>`/`array<empty>`).
- Non-object top-level values and `null` fields reported plainly, no
  guessing.

## Relates to

- `.scratch/boltdb-cli/issues/01-patch-command.md` — knowing the shape of a
  value is a natural precursor to patching specific fields in it.
