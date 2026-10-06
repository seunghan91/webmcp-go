// Package webmcp is a server-side toolkit for WebMCP, the W3C Community Group
// proposal that lets a web page register tools an in-browser AI agent can call
// through document.modelContext (registerTool, getTools, executeTool).
//
// WebMCP is not the server-to-server Model Context Protocol (MCP). An MCP server
// is called by desktop or CLI agents over stdio or HTTP; WebMCP tools live in a
// browser tab and run with the signed-in user's session. This package covers the
// server half of WebMCP for net/http applications, using the standard library only:
//
//   - NewTool validates a tool definition at boot (name 1–128 of [A-Za-z0-9_.-],
//     WebMCP annotations read_only/untrusted_content/consequential/debugging,
//     a strict JSON Schema subset, a same-origin endpoint, GET only when read_only).
//   - Registry.Manifest selects the tools a page exposes; Manifest.ScriptTag
//     renders them as a script-safe <script type="application/json"> element.
//   - RuntimeHandler serves the shared browser runtime (embedded, zero
//     dependencies). It registers the manifest's tools and, when an agent calls
//     one, calls the tool's endpoint with the user's session cookies and CSRF
//     token, without following redirects and without retries.
//   - FormAttrs and ParamAttr render the declarative form attributes
//     (toolname, tooldescription, toolautosubmit, toolparamdescription).
//   - OriginTrial and OriginTrialMetaTag deliver the Chrome origin trial token.
//
// Your endpoints keep authentication, authorization, CSRF checks and input
// validation; annotations are hints, not security controls.
//
// The same manifest format and runtime ship in the Ruby (reference), Django and
// Rust packages: https://github.com/seunghan91/webmcp. Spec baseline: WebMCP
// Draft CG Report 2026-10-02.
package webmcp
