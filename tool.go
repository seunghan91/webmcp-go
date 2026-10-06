package webmcp

import (
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Version is this package's release version.
const Version = "0.2.0"

// Annotations accepts only read_only, untrusted_content, consequential, debugging.
// False hints are omitted from manifests. Hints are not security controls.
type Annotations map[string]bool

// Endpoint describes an existing same-origin application route.
type Endpoint struct {
	Path        string            `json:"path"`
	Method      string            `json:"method"`
	ParamMap    map[string]string `json:"param_map,omitempty"`
	ArrayFormat string            `json:"array_format,omitempty"`
}

// Def defines the browser surface explicitly. Optional pointers distinguish
// absence from an empty title or an invalid zero response limit.
type Def struct {
	Name             string         `json:"name"`
	Description      string         `json:"description"`
	Title            *string        `json:"title,omitempty"`
	InputSchema      map[string]any `json:"input_schema"`
	Endpoint         Endpoint       `json:"endpoint"`
	Annotations      Annotations    `json:"annotations,omitempty"`
	MaxResponseChars *int           `json:"max_response_chars,omitempty"`
	Logger           *slog.Logger   `json:"-"` // nil uses slog.Default for budget warnings.
}

// Tool is a validated, private snapshot; changing its Def cannot change it.
// Construct tools with NewTool; the zero value is not valid.
type Tool struct{ entry map[string]any }

var toolNamePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,128}$`)
var destinationPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)
var annotationNames = map[string]string{
	"read_only": "readOnlyHint", "untrusted_content": "untrustedContentHint",
	"consequential": "consequentialHint", "debugging": "debugging",
}

func validateName(name string) error {
	if !toolNamePattern.MatchString(name) {
		return fmt.Errorf("tool name must be 1..128 characters from A-Z, a-z, 0-9, _, . and -")
	}
	return nil
}

func hasControl(s string) bool {
	for _, r := range s {
		if r < 32 || r == 127 {
			return true
		}
	}
	return false
}

// NewTool validates a definition at boot and makes a deeply copied snapshot.
func NewTool(def Def) (*Tool, error) {
	if err := validateName(def.Name); err != nil {
		return nil, err
	}
	if def.Description == "" || !utf8.ValidString(def.Description) {
		return nil, fmt.Errorf("description must be a nonempty UTF-8 string")
	}
	raw, err := normalize(def.InputSchema)
	if err != nil {
		return nil, err
	}
	schema, err := validateSchema(raw)
	if err != nil {
		return nil, err
	}
	annotations := map[string]any{}
	for key, value := range def.Annotations {
		name, ok := annotationNames[key]
		if !ok {
			return nil, fmt.Errorf("unsupported annotation %q: WebMCP accepts only read_only, untrusted_content, consequential, debugging; MCP hints are not mapped", key)
		}
		if value {
			annotations[name] = true
		}
	}
	endpoint, err := validateEndpoint(def.Endpoint, schema, def.Annotations["read_only"])
	if err != nil {
		return nil, err
	}
	entry := map[string]any{"name": def.Name, "description": def.Description, "inputSchema": schema, "annotations": annotations, "endpoint": endpoint}
	if def.Title != nil {
		if !utf8.ValidString(*def.Title) {
			return nil, fmt.Errorf("title must be valid UTF-8")
		}
		entry["title"] = *def.Title
	}
	if def.MaxResponseChars != nil {
		n := int64(*def.MaxResponseChars)
		if n <= 0 || n > maxInteger {
			return nil, fmt.Errorf("max_response_chars must be a positive integer within +/-2^53")
		}
		entry["maxResponseChars"] = n
	}
	logger := def.Logger
	if logger == nil {
		logger = slog.Default()
	}
	if len(def.Name) > 30 {
		logger.Warn("WebMCP name exceeds Chrome's recommended 30-character budget", "name", def.Name)
	}
	if utf8.RuneCountInString(def.Description) > 500 {
		logger.Warn("WebMCP description exceeds Chrome's recommended 500-character budget", "name", def.Name)
	}
	return &Tool{entry: entry}, nil
}

func validateEndpoint(endpoint Endpoint, schema map[string]any, readOnly bool) (map[string]any, error) {
	path := endpoint.Path
	if !utf8.ValidString(path) || !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") || strings.ContainsAny(path, `\:`) || hasControl(path) {
		return nil, fmt.Errorf("endpoint path must be same-origin, start with /, and contain no // prefix, backslash, colon or control character")
	}
	method := strings.ToUpper(endpoint.Method)
	switch method {
	case "GET", "POST", "PATCH", "PUT", "DELETE":
	default:
		return nil, fmt.Errorf("endpoint method must be GET, POST, PATCH, PUT or DELETE")
	}
	if method == "GET" && !readOnly {
		return nil, fmt.Errorf("GET endpoints require read_only: true")
	}
	properties, _ := schema["properties"].(map[string]any)
	mapping := map[string]any{}
	for key, dest := range endpoint.ParamMap {
		if _, exists := properties[key]; !exists {
			return nil, fmt.Errorf("param_map key is not in schema properties: %s", key)
		}
		if !destinationPattern.MatchString(dest) || reserved(dest) || dest == "_method" || dest == "authenticity_token" || dest == "csrfmiddlewaretoken" {
			return nil, fmt.Errorf("invalid or reserved param_map destination: %q", dest)
		}
		mapping[key] = dest
	}
	seen := map[string]bool{}
	hasArray := false
	for key, raw := range properties {
		dest := key
		if mapped, exists := endpoint.ParamMap[key]; exists {
			dest = mapped
		}
		if seen[dest] {
			return nil, fmt.Errorf("param_map destinations collide")
		}
		seen[dest] = true
		if raw.(map[string]any)["type"] == "array" {
			hasArray = true
		}
	}
	format := endpoint.ArrayFormat
	if format != "" && format != "brackets" && format != "repeat" {
		return nil, fmt.Errorf("array_format must be brackets or repeat")
	}
	if format == "" && method == "GET" && hasArray {
		format = "repeat"
	}
	result := map[string]any{"path": path, "method": method}
	if len(mapping) > 0 {
		result["paramMap"] = mapping
	}
	if format != "" {
		result["arrayFormat"] = format
	}
	return result, nil
}
