# boltdb-cli

A CLI for inspecting and patching bbolt database files, generically and (for Portainer's specific version bucket) schema-aware.

## Language

**Inferred schema**:
The field-name/type shape reported by `get-schema` for a bucket, derived from JSON-decoding one or more sample values rather than any declared or enforced schema — bbolt buckets are schemaless, so this is always a best-effort guess, never a guarantee.
_Avoid_: Schema (unqualified, when talking about a bucket's actual guarantees — bbolt has none)

**Primary sample**:
The single key/value pair `get-schema` decodes to infer a bucket's shape: the bucket's first key via `ListKeys`, or a caller-chosen key via `--key`.
_Avoid_: Sample (unqualified, once a fallback scan is also in play)

**Ambiguous field**:
A field in the primary sample whose inferred type carries no useful information on its own — currently `null` or an empty array (`array<empty>`). Triggers the fallback scan.

**Fallback scan**:
A bounded secondary lookup, triggered only by an ambiguous field, that walks additional keys in the same bucket looking for one where that field resolves to a concrete type. Stops at the first resolution or after a fixed cap on keys checked. Distinct from full shape-drift detection (out of scope for v1): it only resolves specific ambiguous fields, not the whole bucket's shape.
