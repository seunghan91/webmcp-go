# webmcp-go

Framework-agnostic Go helpers for [WebMCP](https://github.com/webmachinelearning/webmcp) —
the W3C proposal that lets web pages declare structured tools for AI agents
(`document.modelContext`).

**Status: early development.** The WebMCP spec is in Chrome origin trial
(Chrome 149–156) and its API surface has already changed twice. This module
currently ships the one piece of server-side plumbing every participating
origin needs — the `Origin-Trial` response header — and stays deliberately
small until the spec settles.

## Install

```bash
go get github.com/seunghan91/webmcp-go
```

## Usage

```go
package main

import (
	"net/http"

	"github.com/seunghan91/webmcp-go"
)

func handler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("ok"))
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", handler)

	wrapped := webmcp.OriginTrial("your-origin-trial-token")(mux)
	http.ListenAndServe(":8080", wrapped)
}
```

If the token is empty, `OriginTrial` returns a pass-through middleware. If
the response already has an `Origin-Trial` header — set by an earlier
middleware in the chain — `OriginTrial` leaves it as-is rather than
overwriting it.

## License

MIT
