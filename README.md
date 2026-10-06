# webmcp-go

0.2.0 · Spec baseline: **WebMCP Draft CG Report 2026-10-02** · Go 1.22+ · Standard library only. Browser test target: Chrome 154 (live verification pending). Go unit results do not establish browser, CSP or production compatibility.

## Intent: share identity, project the rest explicitly

**Surfaces are intentionally different; share identity, project the rest explicitly.**
A server MCP tool and its browser counterpart can represent the same feature
while intentionally having different schemas, limits, and execution paths.

| Difference | Server MCP | Browser WebMCP | Reason |
|---|---|---|---|
| Field names | `content`, `id` | `title`, `task_id` | Browser agents benefit from names matching visible labels. |
| Dates | ISO 8601 | `YYYY-MM-DD HH:MM` in the user's timezone | Match what the user sees. |
| Result limit | 500 | 20 | Keep results useful within the browser's response budget and tab context. |
| Execution path | Service object → database | Session cookies → existing web endpoint | Preserve session, CSRF and authorization checks. |

Identity and descriptions can start from one source. Schema changes, annotations,
limits and endpoints are explicit projections. MCP and WebMCP annotations are
different sets: `destructiveHint` does not automatically mean `consequentialHint`.
Even `readOnlyHint` must be declared again for the browser endpoint.

## Quick start

```sh
go get github.com/seunghan91/webmcp-go
```

Define tools and configure transport at boot. `NewTool` copies metadata; later
changes to the definition cannot alter registered tools. Check all returned errors.

```go
package main

import (
    "html/template"
    "log"
    "net/http"

    webmcp "github.com/seunghan91/webmcp-go"
)

func main() {
    limit := 1500
    tool, err := webmcp.NewTool(webmcp.Def{
        Name: "list_tasks",
        Description: "List at most 20 tasks for the signed-in user.",
        InputSchema: map[string]any{
            "type": "object",
            "properties": map[string]any{
                "tags": map[string]any{
                    "type": "array", "items": map[string]any{"type": "string"},
                },
                "completed": map[string]any{"type": "boolean"},
            },
        },
        Annotations: webmcp.Annotations{"read_only": true, "untrusted_content": true},
        Endpoint: webmcp.Endpoint{Path: "/api/tasks", Method: "GET"},
        MaxResponseChars: &limit,
    })
    if err != nil { log.Fatal(err) }
    registry, err := webmcp.NewRegistry(webmcp.Transport{
        CSRF: &webmcp.CSRF{Source: "meta", Name: "csrf-token", Header: "X-CSRF-Token"},
    })
    if err != nil { log.Fatal(err) }
    if err := registry.Register(tool); err != nil { log.Fatal(err) }
    registry.Freeze()

    page := template.Must(template.New("page").Parse(`<!doctype html>
<html><head><title>Tasks</title></head><body>
{{.Manifest}}{{.Runtime}}
</body></html>`))
    mux := http.NewServeMux()
    mux.Handle("/assets/webmcp-runtime.js", webmcp.RuntimeHandler())
    // Mount your existing authenticated /api/tasks endpoint on mux as well.
    mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        manifest, err := registry.Manifest("list_tasks")
        if err != nil { http.Error(w, err.Error(), 500); return }
        tag, err := manifest.ScriptTag(webmcp.ScriptTagOptions{})
        if err != nil { http.Error(w, err.Error(), 500); return }
        w.Header().Set("Content-Type", "text/html; charset=utf-8")
        if err := page.Execute(w, map[string]any{
            "Manifest": tag,
            "Runtime": webmcp.RuntimeScriptTag("/assets/webmcp-runtime.js", ""),
        }); err != nil { log.Print(err) }
    })
    log.Fatal(http.ListenAndServe(":8080", webmcp.OriginTrial("YOUR_TOKEN")(mux)))
}
```

For writes, your application must render its current CSRF token into an escaped
`<meta name="csrf-token" content="…">`, or explicitly configure `Source: "cookie"`
with the application's cookie name and header. Go has no implicit CSRF defaults;
`Transport{}` contains no CSRF configuration. The copied runtime only checks for
a missing token when CSRF transport is configured. Existing endpoints must always
perform their own CSRF checks.

### Page selection, templates and runtime

`Registry.Manifest(names...)` includes only listed tools in caller order. With no
names it emits `tools: []`. Unknown names and duplicate registrations/selections
return errors. The zero-value registry uses empty transport; `NewRegistry`
validates and snapshots explicit transport. `Freeze()` prevents later registration.

`Def.Title` is an optional `*string` (including an explicitly empty title).
`Def.MaxResponseChars` is an optional positive `*int`. Empty `paramMap` and absent
optionals are omitted; annotations contain only true hints. Manifests implement
the ordinary `encoding/json` struct contract. Their fingerprints include transport.

`Manifest.ScriptTag` defaults to `data-webmcp-autostart`. The embedded runtime
starts when that marker is present, with no inline executable JavaScript.
Pass a request's CSP nonce to both helpers:

```go
manifestTag, err := manifest.ScriptTag(webmcp.ScriptTagOptions{Nonce: nonce})
runtimeTag := webmcp.RuntimeScriptTag("/assets/webmcp-runtime.js", nonce)
// Handle err, then pass both values to html/template.
```

For manual startup, set `Autostart: &autostart` where `autostart := false`, and
import the runtime from an external application module:

```javascript
import { mount } from "/assets/webmcp-runtime.js";
const handle = mount({ selector: "#webmcp-manifest" });
// After replacing the manifest in an SPA:
await handle.refresh();
// When the owning page/component is removed:
handle.dispose();
```

Autostart exposes the handle as `globalThis.WebMCPRuntime.handle` and dispatches
`webmcp:mounted`. Turbo visits trigger refresh automatically. Unsupported browsers
are a no-op. `RuntimeHandler()` serves the embedded file as
`text/javascript; charset=utf-8` with a quoted SHA-256 ETag and conditional GET/HEAD.

### Declarative forms

Register the helpers in `html/template.FuncMap`:

```go
page := template.Must(template.New("form").Funcs(template.FuncMap{
    "formAttrs": webmcp.FormAttrs,
    "paramAttr": webmcp.ParamAttr,
}).Parse(`<form action="/tasks" method="post" {{formAttrs "create_task" "Create a task" false}}>
    <input name="title" {{paramAttr "Task title"}}>
    <button type="submit">Create</button>
</form>`))
```

Add the application's usual CSRF form field. Helpers generate only `toolname`,
`tooldescription`, the boolean `toolautosubmit`, and `toolparamdescription`.
Names follow the same definition rule. Attribute values are escaped even if
converted from a pre-marked-safe `template.HTML` string. Do not interpolate
untrusted input into agent-visible descriptions.

### Origin Trial

`OriginTrial(token)` remains backward compatible. It fills only a missing
`Origin-Trial` header, including when a downstream handler writes the response.
Existing empty headers are preserved. An empty token is a pass-through.

```go
middleware := webmcp.OriginTrialWithOptions(token, webmcp.OriginTrialOptions{
    WarnOnOACOptOut: true,
    Logger: slog.Default(),
})
meta := webmcp.OriginTrialMetaTag(token)
```

The opt-in warning logs once per wrapped handler for `Origin-Agent-Cluster: ?0`.
It never rewrites that header or forces `?1`. The meta helper escapes the token
and returns empty markup for an empty token.

## Security model

The server remains the security boundary. Tools call existing same-origin
endpoints with the current session; those endpoints must enforce authorization,
CSRF, input validation, range limits and result caps. A schema `maximum` is
metadata, not server enforcement. Annotations are hints, not security controls.

- Endpoint paths must begin with `/`; protocol-relative URLs, backslashes, colons
  and control characters are rejected. The runtime also checks the resolved
  origin and uses same-origin mode/credentials with redirects rejected.
- GET requires `read_only: true`; read-only POST is allowed. Other methods need a
  CSRF token read at invocation time. Missing tokens stop the request.
- Only declared input keys are sent. Prototype-related keys are rejected.
  Explicit `param_map` destinations cannot collide or target reserved transport
  fields such as `_method`, `authenticity_token`, or `csrfmiddlewaretoken`.
- JSON embedding escapes `<`, `>`, `&`, U+2028 and U+2029. This prevents script
  breakout, including mixed-case closing tags and HTML comments. Nonces and HTML
  attributes are independently escaped.
- The runtime returns structured success/error envelopes and never retries.
  An ambiguous write result is `unknown_outcome`: verify with the user before
  retrying. A successful write with unreadable/oversized output stays successful
  with `dataOmitted`; an oversized read returns `response_too_large`. Responses
  are never silently truncated.

**Tool metadata is agent-visible, not just display text.** An AI agent reads
`tooldescription` / `toolparamdescription` as part of its instructions for what
the tool does. Do not build these strings from unvalidated user input (profile
fields, query params, uploaded file names, etc.); a user-controlled value rendered
into tool metadata is a prompt-injection vector that can hijack the agent's
behavior. Keep tool names and descriptions as literal strings you write, not
values derived at request time from data a visitor controls. Define tools at boot
and freeze the registry; HTML escaping alone does not prevent prompt injection.

## Conformance

The unchanged [fixtures](conformance/fixtures) are shared with the Ruby reference.
Tests load every fixture and compare parsed manifest entries, including fixed
fingerprints. Expected results are never regenerated by the implementation.
See [conformance/README.md](conformance/README.md) for canonicalization details.

The browser runtime is copied byte-for-byte from the Ruby reference and checked
against [conformance/RUNTIME.sha256](conformance/RUNTIME.sha256). The Go suite also
covers definition validation, immutable snapshots, page selection, XSS vectors,
actual template rendering, HTTP asset serving, and middleware behavior.
When Node is installed, it runs the copied runtime's GET encoder and checks the
result with `url.ParseQuery`; otherwise only that integration test is skipped.

```sh
go vet ./...
go test ./...
```

## Limitations

This is a 0.x subset, not a general JSON Schema rewriting engine. Root schemas
allow `type: "object"`, `properties`, `required`, and `description`. Properties
are `string`, `number`, `integer`, `boolean`, or arrays of those scalar types.
Property metadata supports `enum`, `description`, `default`, `minimum`, `maximum`,
`maxLength`, and `maxItems`. Nested objects/arrays, `$ref`, composition and other
keywords are rejected. Metadata numbers must be integers within +/-2^53; floats
are not accepted. Use integer Go values or `json.Decoder.UseNumber()` when loading
definitions. `number` inputs may still be fractional at runtime. Enum, bounds and
lengths must be enforced by the endpoint.

Names must be 1–128 ASCII letters/digits or `_`, `.`, `-`; descriptions must be
nonempty. `NewTool` warns through `Def.Logger` (or `slog.Default()`) above 30 name
characters or 500 description characters. Only `read_only`, `untrusted_content`,
`consequential`, and `debugging` annotations are accepted. `ParamMap` explicitly
maps browser property names to endpoint parameters.

GET arrays emit `arrayFormat: "repeat"` by default (`tags=a&tags=b`), matching
Go's `url.Values`. Set `Endpoint.ArrayFormat: "brackets"` for endpoints expecting
`tags[]=a&tags[]=b`. Other methods send JSON bodies.

No cross-origin exposure, automatic response truncation, or MCP SDK bridge is
provided. A `from_mcp`-style projection exists in the Ruby reference and is planned
for Go; this phase defines browser tools explicitly and adds no SDK dependency.
The spec and Origin Trial can change. Live browser/CSP verification remains
outside this Go implementation's validation results.

Sibling packages: [Ruby](https://github.com/seunghan91/webmcp),
[Go](https://github.com/seunghan91/webmcp-go),
[Django](https://github.com/seunghan91/webmcp-django),
[Rust](https://github.com/seunghan91/webmcp-rust).

## License

MIT
