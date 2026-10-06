package webmcp

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"reflect"
	"strings"
	"testing"
)

func validDef() Def {
	return Def{Name: "search", Description: "Find tasks", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string"}, "limit": map[string]any{"type": "integer"}}}, Annotations: Annotations{"read_only": true}, Endpoint: Endpoint{Path: "/api/search", Method: "get"}}
}
func ptr[T any](v T) *T                { return &v }
func properties(d *Def) map[string]any { return d.InputSchema["properties"].(map[string]any) }

func TestDefinitionValidation(t *testing.T) {
	cases := []struct {
		name    string
		change  func(*Def)
		message string
	}{
		{"empty name", func(d *Def) { d.Name = "" }, "tool name"},
		{"long name", func(d *Def) { d.Name = strings.Repeat("a", 129) }, "tool name"},
		{"unicode name", func(d *Def) { d.Name = "검색" }, "tool name"},
		{"name spaces", func(d *Def) { d.Name = "a b" }, "tool name"},
		{"name newline", func(d *Def) { d.Name = "a\n" }, "tool name"},
		{"empty description", func(d *Def) { d.Description = "" }, "description"},
		{"invalid UTF8 description", func(d *Def) { d.Description = string([]byte{0xff}) }, "UTF-8"},
		{"invalid UTF8 title", func(d *Def) { d.Title = ptr(string([]byte{0xff})) }, "UTF-8"},
		{"MCP annotation", func(d *Def) { d.Annotations["destructiveHint"] = true }, "unsupported annotation"},
		{"unknown false hint", func(d *Def) { d.Annotations["other"] = false }, "unsupported annotation"},
		{"write GET", func(d *Def) { d.Annotations = nil }, "GET endpoints require"},
		{"invalid method", func(d *Def) { d.Endpoint.Method = "HEAD" }, "method"},
		{"missing method", func(d *Def) { d.Endpoint.Method = "" }, "method"},
		{"absolute endpoint", func(d *Def) { d.Endpoint.Path = "https://evil.test" }, "same-origin"},
		{"protocol relative", func(d *Def) { d.Endpoint.Path = "//evil.test" }, "same-origin"},
		{"backslash", func(d *Def) { d.Endpoint.Path = `/\evil.test` }, "same-origin"},
		{"colon", func(d *Def) { d.Endpoint.Path = "/x:y" }, "same-origin"},
		{"control", func(d *Def) { d.Endpoint.Path = "/x\n" }, "same-origin"},
		{"DEL", func(d *Def) { d.Endpoint.Path = "/x\x7f" }, "same-origin"},
		{"invalid UTF8 path", func(d *Def) { d.Endpoint.Path = "/\xff" }, "same-origin"},
		{"unknown map key", func(d *Def) { d.Endpoint.ParamMap = map[string]string{"missing": "value"} }, "not in schema"},
		{"map collision", func(d *Def) { d.Endpoint.ParamMap = map[string]string{"query": "limit"} }, "collide"},
		{"explicit collision", func(d *Def) { d.Endpoint.ParamMap = map[string]string{"query": "value", "limit": "value"} }, "collide"},
		{"invalid array format", func(d *Def) { d.Endpoint.ArrayFormat = "csv" }, "array_format"},
		{"zero cap", func(d *Def) { d.MaxResponseChars = ptr(0) }, "positive integer"},
		{"negative cap", func(d *Def) { d.MaxResponseChars = ptr(-1) }, "positive integer"},
		{"nil schema", func(d *Def) { d.InputSchema = nil }, "object"},
		{"nonobject schema", func(d *Def) { d.InputSchema["type"] = "array" }, "type must be object"},
		{"unknown root keyword", func(d *Def) { d.InputSchema["additionalProperties"] = false }, "unsupported"},
		{"root description type", func(d *Def) { d.InputSchema["description"] = true }, "description"},
		{"properties type", func(d *Def) { d.InputSchema["properties"] = []any{} }, "properties must be"},
		{"required type", func(d *Def) { d.InputSchema["required"] = "query" }, "required"},
		{"unknown required", func(d *Def) { d.InputSchema["required"] = []string{"missing"} }, "required"},
		{"duplicate required", func(d *Def) { d.InputSchema["required"] = []string{"query", "query"} }, "required"},
		{"required nonstring", func(d *Def) { d.InputSchema["required"] = []any{1} }, "required"},
		{"nested object", func(d *Def) { properties(d)["query"] = map[string]any{"type": "object"} }, "only scalar"},
		{"nil property", func(d *Def) { properties(d)["query"] = nil }, "must be an object"},
		{"ref", func(d *Def) { properties(d)["query"] = map[string]any{"$ref": "x"} }, "unsupported"},
		{"composition", func(d *Def) { d.InputSchema["allOf"] = []any{} }, "unsupported"},
		{"no items", func(d *Def) { properties(d)["query"] = map[string]any{"type": "array"} }, "must be an object"},
		{"nested array", func(d *Def) {
			properties(d)["query"] = map[string]any{"type": "array", "items": map[string]any{"type": "array"}}
		}, "nested arrays"},
		{"scalar items", func(d *Def) { properties(d)["query"].(map[string]any)["items"] = map[string]any{"type": "string"} }, "items requires"},
		{"bad property description", func(d *Def) { properties(d)["query"].(map[string]any)["description"] = 12 }, "description"},
		{"negative maxLength", func(d *Def) { properties(d)["query"].(map[string]any)["maxLength"] = -1 }, "nonnegative"},
		{"float metadata", func(d *Def) { properties(d)["limit"].(map[string]any)["maximum"] = 1.0 }, "floats"},
		{"integer overflow", func(d *Def) { properties(d)["limit"].(map[string]any)["maximum"] = int64(1<<53) + 1 }, "+/-2^53"},
		{"uint overflow", func(d *Def) { properties(d)["limit"].(map[string]any)["maximum"] = uint64(1<<53) + 1 }, "+/-2^53"},
		{"float json number", func(d *Def) { properties(d)["limit"].(map[string]any)["maximum"] = json.Number("1.0") }, "integers"},
		{"empty enum", func(d *Def) { properties(d)["query"].(map[string]any)["enum"] = []any{} }, "nonempty"},
		{"wrong enum type", func(d *Def) { properties(d)["query"].(map[string]any)["enum"] = []any{1} }, "enum must match"},
		{"wrong default", func(d *Def) { properties(d)["query"].(map[string]any)["default"] = nil }, "default must match"},
		{"non JSON", func(d *Def) { properties(d)["query"].(map[string]any)["default"] = func() {} }, "JSON values"},
		{"cycle", func(d *Def) { d.InputSchema["cycle"] = d.InputSchema }, "circular"},
		{"slice cycle", func(d *Def) { a := make([]any, 1); a[0] = a; d.InputSchema["cycle"] = a }, "circular"},
	}
	for _, name := range []string{"__proto__", "constructor", "prototype"} {
		name := name
		cases = append(cases, struct {
			name    string
			change  func(*Def)
			message string
		}{"reserved property " + name, func(d *Def) { properties(d)[name] = map[string]any{"type": "string"} }, "reserved property"})
	}
	for _, dest := range []string{"_method", "authenticity_token", "csrfmiddlewaretoken", "__proto__", "constructor", "prototype", "1bad", "x-y", "a\n", ""} {
		dest := dest
		cases = append(cases, struct {
			name    string
			change  func(*Def)
			message string
		}{"map destination " + dest, func(d *Def) { d.Endpoint.ParamMap = map[string]string{"query": dest} }, "destination"})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := validDef()
			tc.change(&d)
			_, err := NewTool(d)
			if err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("got %v; want %q", err, tc.message)
			}
		})
	}
}

func TestSupportedDefinitions(t *testing.T) {
	for _, method := range []string{"GET", "post", "PATCH", "PUT", "DELETE"} {
		d := validDef()
		d.Endpoint.Method = method
		d.InputSchema["required"] = []string{"query"}
		properties(&d)["limit"] = map[string]any{"type": "number", "minimum": -maxInteger, "maximum": maxInteger, "default": int64(2), "enum": []int{1, 2}, "description": "Cap"}
		properties(&d)["tags"] = map[string]any{"type": "array", "items": map[string]any{"type": "boolean"}, "maxItems": 0, "default": []bool{}, "enum": [][]bool{{true}, {false}}}
		if _, err := NewTool(d); err != nil {
			t.Fatalf("%s: %v", method, err)
		}
	}
	d := validDef()
	d.InputSchema = map[string]any{"type": "object"}
	d.Title = ptr("")
	d.MaxResponseChars = ptr(1)
	if _, err := NewTool(d); err != nil {
		t.Fatal(err)
	}
}

func TestBudgetWarnings(t *testing.T) {
	var output bytes.Buffer
	d := validDef()
	d.Name = strings.Repeat("a", 31)
	d.Description = strings.Repeat("한", 501)
	d.Logger = slog.New(slog.NewTextHandler(&output, nil))
	if _, err := NewTool(d); err != nil {
		t.Fatal(err)
	}
	if strings.Count(output.String(), "level=WARN") != 2 {
		t.Fatalf("warnings: %s", output.String())
	}
	output.Reset()
	d.Name = strings.Repeat("a", 30)
	d.Description = strings.Repeat("한", 500)
	if _, err := NewTool(d); err != nil {
		t.Fatal(err)
	}
	if output.Len() != 0 {
		t.Fatal("budget boundary warned")
	}
}

func TestRegistrySnapshotsAndSelection(t *testing.T) {
	csrf := &CSRF{Source: "meta", Name: "csrf-token", Header: "X-CSRF-Token"}
	registry, err := NewRegistry(Transport{CSRF: csrf})
	if err != nil {
		t.Fatal(err)
	}
	d := validDef()
	d.Endpoint.ParamMap = map[string]string{"query": "q"}
	d.Title = ptr("")
	d.Annotations["debugging"] = false
	tool, err := NewTool(d)
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(tool); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(tool); err == nil {
		t.Fatal("duplicate allowed")
	}
	for _, invalid := range []*Tool{nil, {}} {
		if err := registry.Register(invalid); err == nil {
			t.Fatal("invalid tool allowed")
		}
	}
	before, err := registry.Manifest(d.Name)
	if err != nil {
		t.Fatal(err)
	}
	csrf.Name = "changed"
	properties(&d)["query"].(map[string]any)["type"] = "boolean"
	d.Endpoint.ParamMap["query"] = "changed"
	*d.Title = "changed"
	d.Annotations["read_only"] = false
	after, err := registry.Manifest(d.Name)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("definition mutation changed manifest")
	}
	after.Tools[0]["description"] = "changed"
	after.Transport["csrf"].(map[string]any)["name"] = "changed"
	again, err := registry.Manifest(d.Name)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, again) {
		t.Fatal("manifest mutation changed registry")
	}
	empty, err := registry.Manifest()
	if err != nil {
		t.Fatal(err)
	}
	if empty.Tools == nil || len(empty.Tools) != 0 {
		t.Fatal("default exposed tools")
	}
	for _, names := range [][]string{{"missing"}, {d.Name, d.Name}} {
		if _, err := registry.Manifest(names...); err == nil {
			t.Fatal("invalid selection allowed")
		}
	}
	entry := before.Tools[0]
	if entry["title"] != "" {
		t.Fatal("empty optional title omitted")
	}
	if _, ok := entry["maxResponseChars"]; ok {
		t.Fatal("absent cap emitted")
	}
	if _, ok := entry["annotations"].(map[string]any)["debugging"]; ok {
		t.Fatal("false hint emitted")
	}
	other, err := NewRegistry(Transport{})
	if err != nil {
		t.Fatal(err)
	}
	if err := other.Register(tool); err != nil {
		t.Fatal(err)
	}
	changed, err := other.Manifest(d.Name)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Tools[0]["fingerprint"] == entry["fingerprint"] {
		t.Fatal("transport omitted from fingerprint")
	}
}

func TestTransportValidation(t *testing.T) {
	for _, c := range []CSRF{{Source: "other", Name: "x", Header: "X"}, {Source: "meta", Header: "X"}, {Source: "meta", Name: "x"}, {Source: "cookie", Name: "x\n", Header: "X"}, {Source: "meta", Name: "x", Header: "X\x7f"}, {Source: "meta", Name: "\xff", Header: "X"}} {
		if _, err := NewRegistry(Transport{CSRF: &c}); err == nil {
			t.Fatalf("accepted %#v", c)
		}
	}
	for _, source := range []string{"meta", "cookie"} {
		if _, err := NewRegistry(Transport{CSRF: &CSRF{Source: source, Name: "token", Header: "X-CSRF"}}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRegistryFreezeAndOrder(t *testing.T) {
	registry := &Registry{}
	for _, name := range []string{"first", "second"} {
		d := validDef()
		d.Name = name
		tool, err := NewTool(d)
		if err != nil {
			t.Fatal(err)
		}
		if err := registry.Register(tool); err != nil {
			t.Fatal(err)
		}
	}
	registry.Freeze()
	manifest, err := registry.Manifest("second", "first")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Tools[0]["name"] != "second" || manifest.Tools[1]["name"] != "first" {
		t.Fatal("caller order lost")
	}
	d := validDef()
	tool, err := NewTool(d)
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(tool); err == nil || !strings.Contains(err.Error(), "frozen") {
		t.Fatalf("frozen register: %v", err)
	}
}
