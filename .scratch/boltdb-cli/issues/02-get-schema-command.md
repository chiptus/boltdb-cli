Status: needs-triage

# `get-schema <bucket>` command (agent-facing shape introspection)

## Problem

bbolt has no schema — a bucket is just an opaque list of key/value byte
pairs. An agent (or a human) inspecting an unfamiliar `.db` file has to run
`get` on individual keys and eyeball the raw JSON to figure out what fields
exist before it can safely `put`/`patch` anything. There's no single command
that answers "what does a value in this bucket look like?"

## Proposed shape

`boltdb-cli get-schema <bucket>`

- Picks a sample key from the bucket (first key via `ListKeys`, or a
  `--key` flag to pick a specific one).
- JSON-decodes the sampled value.
- Prints field names and inferred types (string/number/bool/array/nested
  object), recursing into nested objects.
- Output explicitly labeled as inferred from a sample (e.g. `(inferred from
  key "...", 1 sample)`), since it is not a real schema — other keys in the
  same bucket could have different shapes.

## Open questions (candidates for grilling)

- What happens when the sampled value isn't JSON (plain text, uint64-be,
  binary)? Report "not JSON" rather than erroring or printing garbage.
- Should it ever sample more than one key to detect shape drift across a
  bucket, or is single-sample + explicit caveat sufficient for v1?
- Output format: human-readable tree vs. something an agent can parse
  reliably (e.g. also emit `--format json` describing the inferred shape)?
- Does this belong at the generic top level (like `list-buckets`/`list-
  keys`), or does it only make sense generically at all given it's JSON-
  specific introspection?

## Relates to

- `.scratch/boltdb-cli/issues/01-patch-command.md` — knowing the shape of a
  value is a natural precursor to patching specific fields in it.
