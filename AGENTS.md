# AGENTS.md — webmcp-go (Go, net/http)

Guidance for AI coding agents that add this package to an application or work
on this repository. Humans: the [README](README.md) has the same facts with more
explanation.

## What this is

Server-side WebMCP toolkit for Go net/http, standard library only: validated tool definitions, per-page script-safe manifests for html/template, the shared browser runtime via embed, declarative form attribute helpers and Origin Trial middleware.

WebMCP is not the Model Context Protocol (MCP) server protocol. An MCP server is
called by desktop or CLI agents (Claude Desktop, Claude Code, ChatGPT connectors)
over stdio or HTTP. WebMCP tools are registered by a web page with
`document.modelContext.registerTool()` and are called by an AI agent running inside
the user's browser tab, with that user's session. Use an MCP SDK for the first,
this package for the second; they can share tool identity (see the README's
"Intent" section).

## When to use it

**Use it when** a server-rendered or hybrid web app (Rails, Django, Go net/http,
Rust web frameworks, Inertia/Turbo SPAs) should expose some of its existing
same-origin JSON endpoints as WebMCP tools for in-browser agents, and you want
validation at boot, page-level opt-in, CSRF-aware calls and safe HTML embedding.

**Do not use it when** you need an MCP server for desktop/CLI agents (use an MCP
SDK), cross-origin tool exposure (`exposedTo`/`fromOrigins`, not supported in 0.x),
or a purely client-side app with no server templates (call
`document.modelContext.registerTool()` directly or use an npm library such as
MCP-B).

## Integration recipe (Go)

1. Install. `go get github.com/seunghan91/webmcp-go@latest` and import it as `webmcp "github.com/seunghan91/webmcp-go"`. Go >= 1.22, standard library only.
2. Pick existing same-origin JSON endpoints to expose. Prefer a few read tools
   first; add write tools only where the endpoint already enforces CSRF.
3. Define each tool at boot with a literal name (≤ 30 characters recommended,
   `[A-Za-z0-9_.-]`), a description an agent can act on (≤ 500 characters
   recommended), an input schema in the 0.x subset, WebMCP annotations and the
   endpoint path and method.
4. Render the manifest on the pages that should expose those tools, the runtime
   script tag, and a matching CSRF token for every non-GET tool (including
   read-only POST tools). With a nonce-based CSP, pass the response's nonce to
   the script helpers (Rails `content_security_policy_nonce`, Django
   `request.csp_nonce`, Go `ScriptTagOptions.Nonce` / `RuntimeScriptTag`, Rust
   `ScriptTagOptions.nonce` / `runtime_script_tag`); the runtime needs no inline
   script. The CSP must also allow the runtime's same-origin endpoint requests
   (`connect-src 'self'`, or `default-src 'self'` without a stricter
   `connect-src`). Turbo visits are handled automatically, including a first page
   without a manifest. In other SPAs, after the page replaces or removes
   `#webmcp-manifest`, call `await WebMCPRuntime.handle.refresh()`
   (`webmcp:mounted` only delivers the initial handle). If such an SPA's first
   page has no manifest, render the manifest with autostart off and start the
   runtime from your own module instead:
   `import { mount } from "<runtime URL>"; const handle = mount();` — then call
   `handle.refresh()` after each navigation.
5. Verify: enable `chrome://flags/#enable-webmcp-testing` (or the origin trial),
   open the page over HTTPS or localhost, and run in DevTools:

   ```js
   await WebMCPRuntime.handle.refresh();
   const tool = (await document.modelContext.getTools())
     .find(tool => tool.name === "list_tasks");
   if (!tool) throw new Error("list_tasks was not registered");
   const input = JSON.stringify({ completed: false }); // Chrome 154 needs a string
   // Chrome 155+: const input = { completed: false };
   const outcome = JSON.parse(await document.modelContext.executeTool(tool, input));
   console.log(outcome); // { ok: true, status: 200, data: ... }
   ```

Run this in your startup code (import `net/http` and the package as `webmcp`):

```go
mux := http.NewServeMux() // or your existing *http.ServeMux
tool, err := webmcp.NewTool(webmcp.Def{
    Name:        "list_tasks",
    Description: "List at most 20 tasks for the signed-in user.",
    InputSchema: map[string]any{"type": "object",
        "properties": map[string]any{"completed": map[string]any{"type": "boolean"}}},
    Annotations: webmcp.Annotations{"read_only": true, "untrusted_content": true},
    Endpoint:    webmcp.Endpoint{Path: "/api/tasks", Method: "GET"}, // existing JSON endpoint
})
if err != nil {
    panic(err)
}
registry, err := webmcp.NewRegistry(webmcp.Transport{
    CSRF: &webmcp.CSRF{Source: "meta", Name: "csrf-token", Header: "X-CSRF-Token"},
})
if err != nil {
    panic(err)
}
if err := registry.Register(tool); err != nil {
    panic(err)
}
registry.Freeze()
mux.Handle("/assets/webmcp-runtime.js", webmcp.RuntimeHandler())
// per request: manifest, _ := registry.Manifest("list_tasks")
//              tag, _ := manifest.ScriptTag(webmcp.ScriptTagOptions{Nonce: nonce})
//              runtime := webmcp.RuntimeScriptTag("/assets/webmcp-runtime.js", nonce)
// pass tag and runtime to html/template (both are template.HTML). Check every error.
```

- Go has no implicit CSRF default: configure `Transport.CSRF` and render the token into `<meta name="csrf-token" content="...">` yourself (escaped), or use `Source: "cookie"`.
- Runnable examples: `Example` and `ExampleNewTool_getRequiresReadOnly` in `example_test.go` (shown on pkg.go.dev).

The runtime returns an envelope instead of throwing: `{ ok: true, status, data }`
or `{ ok: false, status?, error: { code, message } }`, with codes
`invalid_input`, `csrf_token_missing`, `http_error`, `network_error`, `aborted`,
`invalid_response`, `response_too_large`, `unknown_outcome`. Successful writes
whose body cannot be returned carry `dataOmitted`.

## Invariants — do not violate

1. Every tool calls an **existing same-origin JSON endpoint** that already enforces
   authentication, authorization, CSRF and input validation. The toolkit adds no
   permission path; annotations are hints, not security controls.
2. `GET` endpoints must be read-only and the tool must declare `read_only`. Reads
   may use POST. Write endpoints should answer JSON on every path (return 401/403
   JSON, never a 200 HTML sign-in page — a write tool reads 2xx non-JSON as success
   with omitted data).
3. Tool metadata (names, descriptions, parameter descriptions) is part of the
   agent's instructions. Write it as literals; never interpolate request or user
   data into it (prompt-injection vector).
4. Expose tools per page: render the manifest only on pages that list them.
   Registering a tool does not expose it.
5. WebMCP annotations are `read_only`, `untrusted_content`, `consequential`,
   `debugging`. MCP's `destructiveHint`/`idempotentHint`/`openWorldHint` are a
   different set and are never mapped automatically.
6. Mark tools that return user-generated or external text `untrusted_content`;
   mark irreversible actions (payments, deletion, sending) `consequential`.
7. Input schemas use the 0.x subset: top-level `type: "object"` with scalar or
   array-of-scalar properties (`string`, `number`, `integer`, `boolean`) and
   `enum`/`description`/`default`/`minimum`/`maximum`/`maxLength`/`maxItems`.
   Nested objects, `$ref` and combinators are rejected at definition time. This
   is a package restriction, not the full WebMCP contract. Numeric metadata
   (bounds, defaults, enum values) must be integers within ±2^53; runtime number
   inputs may be fractional. The runtime checks declared keys, required keys and
   scalar/array types only; endpoints enforce enums, bounds and lengths and apply
   defaults.
8. Do not edit the vendored browser runtime; it is shared byte-for-byte across
   the four packages.

## Troubleshooting

See the README's [Troubleshooting](README.md#troubleshooting) table (exact error
messages, causes and fixes).

## Working on this repository

- Verify: `go vet ./... && go test -race ./...`.
- `runtime/webmcp-runtime.js` is a byte-for-byte copy of the canonical runtime in github.com/seunghan91/webmcp; do not edit it here. `conformance/RUNTIME.sha256` pins it and a test checks it.
- `conformance/fixtures/*.json` are shared with the Ruby reference; keep them identical.
- Spec facts used by all four packages are summarized in the Ruby reference README and come from the WebMCP Draft CG Report 2026-10-02.

## Related packages

- [webmcp (Ruby / Rails, reference)](https://github.com/seunghan91/webmcp): RubyGems `webmcp`
- [webmcp-go (Go)](https://github.com/seunghan91/webmcp-go): `github.com/seunghan91/webmcp-go`
- [webmcp-django (Python / Django)](https://github.com/seunghan91/webmcp-django): PyPI `webmcp-django`
- [webmcp-rust (Rust)](https://github.com/seunghan91/webmcp-rust): crates.io `webmcp`
