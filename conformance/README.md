# Manifest v1 conformance

`fixtures/*.json` are verbatim copies of the Ruby reference fixtures. Each has
`definition` (snake_case API inputs), `transport`, and `expected` (one tool entry).
Compare parsed JSON, including the fixed fingerprint, rather than serialized key
order. Expected fingerprints were independently calculated using Python's sorted,
compact UTF-8 JSON and SHA-256; do not regenerate them with the implementation.

The effective fingerprint preimage is the complete tool entry without
`fingerprint`, plus a `transport` member containing the manifest transport.
Recursively sort object keys, preserve array order, serialize compact UTF-8 JSON
without HTML escaping, hash with SHA-256, and prefix lowercase hex with `sha256:`.
Go's encoder sorts map keys; struct fields are represented as maps for hashing.
Canonical strings retain literal U+2028/U+2029; JSON embedded in HTML escapes them.
Literal backslash-u sequences are preserved in both forms.

Metadata numbers are integers within +/-2^53. Absent optional fields and empty
`paramMap` are omitted; empty `annotations` is retained. Go GET arrays default to
`repeat`, while the shared read fixture explicitly requests `brackets` to avoid
any language-dependent expected result. `projection-result.json` describes the
result of a Ruby projection and is usable without an MCP SDK bridge.

`RUNTIME.sha256` pins `runtime/webmcp-runtime.js`, copied without modification from
the Ruby reference. Copy the runtime and its hash record together on updates.
The tests compare the file, embedded bytes and recorded hash, and check HTTP
content type, ETag, HEAD and conditional responses.
