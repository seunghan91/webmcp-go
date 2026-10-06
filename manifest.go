package webmcp

import (
	"encoding/json"
	"fmt"
	"html/template"
	"sync"
	"unicode/utf8"
)

// CSRF locates a fresh token at invocation time. Source is meta or cookie.
type CSRF struct {
	Source string `json:"source"`
	Name   string `json:"name"`
	Header string `json:"header"`
}

// Transport has no framework defaults. Configure CSRF explicitly for writes.
type Transport struct {
	CSRF *CSRF `json:"csrf,omitempty"`
}

func (t Transport) normalized() (map[string]any, error) {
	result := map[string]any{}
	if t.CSRF != nil {
		c := t.CSRF
		if (c.Source != "meta" && c.Source != "cookie") || c.Name == "" || c.Header == "" || hasControl(c.Name) || hasControl(c.Header) || !utf8.ValidString(c.Name) || !utf8.ValidString(c.Header) {
			return nil, fmt.Errorf("csrf requires source meta/cookie and nonempty UTF-8 name/header without control characters")
		}
		// Build maps, not structs, so fingerprint key sorting is recursive.
		result["csrf"] = map[string]any{"source": c.Source, "name": c.Name, "header": c.Header}
	}
	return result, nil
}

// Registry stores validated tools. Its zero value uses empty transport.
// Register at boot; Manifest only exposes explicitly requested names.
// A Registry must not be copied after first use.
type Registry struct {
	mu        sync.RWMutex
	tools     map[string]*Tool
	transport map[string]any
	frozen    bool
}

// NewRegistry validates and snapshots the transport configuration.
func NewRegistry(transport Transport) (*Registry, error) {
	value, err := transport.normalized()
	if err != nil {
		return nil, err
	}
	return &Registry{transport: value}, nil
}

// Register adds a tool, rejecting invalid tools and duplicate names.
func (r *Registry) Register(tool *Tool) error {
	if tool == nil || tool.entry == nil {
		return fmt.Errorf("tool must be constructed with NewTool")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.frozen {
		return fmt.Errorf("registry is frozen")
	}
	if r.tools == nil {
		r.tools = make(map[string]*Tool)
	}
	name := tool.entry["name"].(string)
	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("duplicate tool name: %s", name)
	}
	r.tools[name] = tool
	return nil
}

// Freeze prevents further registration after application initialization.
func (r *Registry) Freeze() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.frozen = true
}

// Manifest is a manifest v1 snapshot. Marshal it with encoding/json or render
// it with ScriptTag. Mutating a snapshot never changes the registry or tools.
type Manifest struct {
	WebMCPManifestVersion int              `json:"webmcpManifestVersion"`
	Transport             map[string]any   `json:"transport"`
	Tools                 []map[string]any `json:"tools"`
}

// Manifest selects exactly names in caller order. No names means no tools.
func (r *Registry) Manifest(names ...string) (Manifest, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	transport := r.transport
	if transport == nil {
		transport = map[string]any{}
	}
	copied, err := normalize(transport)
	if err != nil {
		return Manifest{}, err
	}
	manifest := Manifest{WebMCPManifestVersion: 1, Transport: copied.(map[string]any), Tools: make([]map[string]any, 0, len(names))}
	seen := map[string]bool{}
	for _, name := range names {
		if seen[name] {
			return Manifest{}, fmt.Errorf("duplicate tool name in manifest: %s", name)
		}
		seen[name] = true
		tool, exists := r.tools[name]
		if !exists {
			return Manifest{}, fmt.Errorf("unknown tool name: %s", name)
		}
		copied, err := normalize(tool.entry)
		if err != nil {
			return Manifest{}, err
		}
		entry := copied.(map[string]any)
		entry["transport"] = manifest.Transport
		hash, err := fingerprint(entry)
		if err != nil {
			return Manifest{}, err
		}
		delete(entry, "transport")
		entry["fingerprint"] = hash
		manifest.Tools = append(manifest.Tools, entry)
	}
	return manifest, nil
}

// ScriptTagOptions controls CSP and page startup. Autostart nil defaults to true;
// a pointer to false omits data-webmcp-autostart.
type ScriptTagOptions struct {
	Nonce     string
	Autostart *bool
}

// ScriptTag embeds JSON safely, including HTML-significant characters and
// U+2028/U+2029. It does not include inline executable JavaScript.
func (m Manifest) ScriptTag(opts ScriptTagOptions) (template.HTML, error) {
	// Re-normalize public snapshots to reject custom marshalers and unsafe metadata.
	value, err := normalize(map[string]any{"webmcpManifestVersion": m.WebMCPManifestVersion, "transport": m.Transport, "tools": m.Tools})
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	attrs := nonceAttr(opts.Nonce)
	if opts.Autostart == nil || *opts.Autostart {
		attrs += " data-webmcp-autostart"
	}
	return template.HTML(`<script type="application/json" id="webmcp-manifest"` + attrs + `>` + string(data) + `</script>`), nil
}
