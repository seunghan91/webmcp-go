package webmcp

import (
	"crypto/sha256"
	_ "embed"
	"fmt"
	"net/http"
	"strings"
	"time"
)

//go:embed runtime/webmcp-runtime.js
var runtimeSource string

// RuntimeHandler serves the pinned browser runtime, with a strong SHA-256 ETag.
// Register it at the URL passed to RuntimeScriptTag.
func RuntimeHandler() http.Handler {
	etag := fmt.Sprintf(`"%x"`, sha256.Sum256([]byte(runtimeSource)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Header().Set("ETag", etag)
		http.ServeContent(w, r, "webmcp-runtime.js", time.Time{}, strings.NewReader(runtimeSource))
	})
}
