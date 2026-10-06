package webmcp

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestConformanceFixtures(t *testing.T) {
	files, err := filepath.Glob("conformance/fixtures/*.json")
	if err != nil || len(files) == 0 {
		t.Fatalf("fixtures: %v, %v", files, err)
	}
	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			var fixture struct {
				Definition Def            `json:"definition"`
				Transport  Transport      `json:"transport"`
				Expected   map[string]any `json:"expected"`
			}
			decoder := json.NewDecoder(bytes.NewReader(data))
			decoder.UseNumber()
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&fixture); err != nil {
				t.Fatal(err)
			}
			tool, err := NewTool(fixture.Definition)
			if err != nil {
				t.Fatal(err)
			}
			registry, err := NewRegistry(fixture.Transport)
			if err != nil {
				t.Fatal(err)
			}
			if err := registry.Register(tool); err != nil {
				t.Fatal(err)
			}
			manifest, err := registry.Manifest(fixture.Definition.Name)
			if err != nil {
				t.Fatal(err)
			}
			actual, err := normalize(manifest.Tools[0])
			if err != nil {
				t.Fatal(err)
			}
			expected, err := normalize(fixture.Expected)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(actual, expected) {
				t.Fatalf("entry mismatch\ngot: %#v\nwant: %#v", actual, expected)
			}
			if manifest.WebMCPManifestVersion != 1 {
				t.Fatal("wrong manifest version")
			}
		})
	}
}

func TestCanonicalJSON(t *testing.T) {
	value := map[string]any{
		"z": "</ScRiPt><!-->&\u2028\u2029한글", "a": map[string]any{"z": 2, "a": 1},
		"literal": `\u2028\u2029`, "mixed": "\\\u2028", "array": []any{2, 1},
	}
	got, err := canonicalJSON(value)
	if err != nil {
		t.Fatal(err)
	}
	want := "{\"a\":{\"a\":1,\"z\":2},\"array\":[2,1],\"literal\":\"\\\\u2028\\\\u2029\",\"mixed\":\"\\\\\u2028\",\"z\":\"</ScRiPt><!-->&\u2028\u2029한글\"}"
	if string(got) != want {
		t.Fatalf("canonical preimage\ngot  %q\nwant %q", got, want)
	}
	var roundtrip map[string]any
	if err := json.Unmarshal(got, &roundtrip); err != nil {
		t.Fatal(err)
	}
	if roundtrip["literal"] != value["literal"] || roundtrip["mixed"] != value["mixed"] {
		t.Fatal("backslash corruption")
	}
}

func TestRuntimeDigestAndHandler(t *testing.T) {
	record, err := os.ReadFile("conformance/RUNTIME.sha256")
	if err != nil {
		t.Fatal(err)
	}
	fields := strings.Fields(string(record))
	if len(fields) != 2 || fields[1] != "runtime/webmcp-runtime.js" {
		t.Fatalf("invalid hash record %q", record)
	}
	data, err := os.ReadFile(fields[1])
	if err != nil {
		t.Fatal(err)
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(data))
	if digest != fields[0] || string(data) != runtimeSource {
		t.Fatal("runtime differs from pinned reference")
	}
	handler := RuntimeHandler()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/assets/runtime.js", nil))
	if rec.Code != 200 || rec.Body.String() != string(data) {
		t.Fatal("runtime response mismatch")
	}
	if rec.Header().Get("Content-Type") != "text/javascript; charset=utf-8" {
		t.Fatal("wrong content type")
	}
	if rec.Header().Get("ETag") != `"`+digest+`"` {
		t.Fatal("wrong ETag")
	}
	for _, method := range []string{"GET", "HEAD"} {
		request := httptest.NewRequest(method, "/assets/runtime.js", nil)
		request.Header.Set("If-None-Match", rec.Header().Get("ETag"))
		cached := httptest.NewRecorder()
		handler.ServeHTTP(cached, request)
		if cached.Code != http.StatusNotModified || cached.Body.Len() != 0 {
			t.Fatalf("conditional %s: %d", method, cached.Code)
		}
	}
	head := httptest.NewRecorder()
	handler.ServeHTTP(head, httptest.NewRequest("HEAD", "/assets/runtime.js", nil))
	if head.Code != 200 || head.Body.Len() != 0 {
		t.Fatal("HEAD returned body")
	}
}

func TestRuntimeGETArrayRoundTrip(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node is needed only for the copied runtime query round-trip test")
	}
	def := validDef()
	def.InputSchema = map[string]any{"type": "object", "properties": map[string]any{"tags": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}}}
	tool, err := NewTool(def)
	if err != nil {
		t.Fatal(err)
	}
	registry := &Registry{}
	if err := registry.Register(tool); err != nil {
		t.Fatal(err)
	}
	manifest, err := registry.Manifest(def.Name)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(manifest.Tools[0])
	if err != nil {
		t.Fatal(err)
	}
	script := `import { readFileSync } from 'node:fs';
const source = readFileSync('runtime/webmcp-runtime.js', 'utf8');
const { encodeQuery } = await import('data:text/javascript;base64,' + Buffer.from(source).toString('base64'));
const tool = JSON.parse(readFileSync(0, 'utf8'));
process.stdout.write(encodeQuery(tool, { tags: ['a b', '한글&x', 'a b'] }));`
	cmd := exec.Command(node, "--input-type=module", "-e", script)
	cmd.Stdin = bytes.NewReader(data)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, output)
	}
	query, err := url.ParseQuery(string(output))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(query["tags"], []string{"a b", "한글&x", "a b"}) {
		t.Fatalf("round trip: %v", query)
	}
}
