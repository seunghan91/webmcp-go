# Changelog

## 0.2.0 — 2026-10-06

- Add validated immutable tool definitions, explicit page-selected registries and
  manifest v1 output with portable SHA-256 fingerprints and transport validation.
- Add safe manifest/runtime/meta tags and declarative form/parameter attributes.
- Embed the shared browser runtime and serve it with a SHA-256 ETag.
- Preserve the OriginTrial API and fill headers at response write time, retaining
  downstream and explicitly empty values. Add opt-in, once-only OAC warnings.
- Add shared conformance fixtures, XSS and template tests, definition and HTTP
  regressions, and a runtime GET-array round trip through Go's query parser.
- Document explicit browser projections, the security model and 0.x limitations.
  No third-party modules or MCP SDK bridge are introduced.
