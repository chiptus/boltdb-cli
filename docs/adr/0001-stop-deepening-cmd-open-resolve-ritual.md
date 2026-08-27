# Stop deepening cmd/'s open→resolve-key→tx ritual past two seams

`cmd/dbopen.go` already gives every command two seams: `openReadDB`/`openWriteDB`
(path resolution + open/close) and `resolveKey`/`resolveKeyFormat` (key-format
guess + decode). Commits c7e591f and 2d2b9d5 did this deepening and routed
every command, including the portainer subcommands, through it.

What's left in each `RunE` — `defer db.Close(); db.View(func(tx) {...})` — looks
like duplication but isn't reducible without a worse interface: `get`/`patch`
resolve the key then apply a custom existence-check error, `put` resolves the
key in a `View` before a separate `Update`, `get-schema` resolves the key only
when `--key` is set, and `list-buckets`/`list-keys` skip key resolution
entirely. A single `WithResolvedKey(bucket, rawKey, keyFormat, fn)` wrapper
would either save a few lines while adding an indirection that fails the
deletion test (deleting it just unfolds the differing bodies back to each call
site), or sprout options/flags to cover every variant — a shallower interface
than the current two-seam split.

Decision: treat this as done. Don't re-propose collapsing the remaining
per-command `db.View` wrapping in future architecture reviews unless a new
command's body reveals a genuinely shared sub-sequence beyond open+resolve.
