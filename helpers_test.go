package webmcp

import (
	"bytes"
	"encoding/json"
	"html"
	"html/template"
	"strings"
	"testing"
)

func TestScriptTagXSS(t *testing.T) {
	for _, vector := range []string{`</ScRiPt><script>alert(1)</script>`, `<!--`, `&`, "\u2028", "\u2029", `" onload="alert(1)`, `\u2028`} {
		t.Run(vector, func(t *testing.T) {
			d := validDef()
			d.Description = vector
			// Deliberately pre-mark metadata as trusted HTML; JSON still must escape it.
			properties(&d)["query"].(map[string]any)["description"] = template.HTML(vector)
			tool, err := NewTool(d)
			if err != nil {
				t.Fatal(err)
			}
			registry := &Registry{}
			if err := registry.Register(tool); err != nil {
				t.Fatal(err)
			}
			manifest, err := registry.Manifest(d.Name)
			if err != nil {
				t.Fatal(err)
			}
			tag, err := manifest.ScriptTag(ScriptTagOptions{Nonce: `"><script>&`})
			if err != nil {
				t.Fatal(err)
			}
			text := string(tag)
			start := strings.Index(text, ">") + 1
			body := text[start : len(text)-len("</script>")]
			if strings.ContainsAny(body, "<>&\u2028\u2029") {
				t.Fatalf("unsafe body %q", body)
			}
			if strings.Count(strings.ToLower(text), "</script>") != 1 {
				t.Fatal("script breakout")
			}
			if !strings.Contains(text, `nonce="&#34;&gt;&lt;script&gt;&amp;"`) {
				t.Fatalf("nonce escaping: %s", text)
			}
			var decoded struct {
				Tools []struct {
					Description string
					InputSchema map[string]any
				}
			}
			if err := json.Unmarshal([]byte(body), &decoded); err != nil {
				t.Fatal(err)
			}
			if decoded.Tools[0].Description != vector {
				t.Fatal("JSON changed input")
			}
			property := decoded.Tools[0].InputSchema["properties"].(map[string]any)["query"].(map[string]any)
			if property["description"] != vector {
				t.Fatal("safe HTML changed input")
			}
		})
	}
}

func TestTemplateHelpers(t *testing.T) {
	vector := template.HTML(`"><img src=x onerror='alert(1)'>&`)
	attrs, err := FormAttrs("create_task", string(vector), true)
	if err != nil {
		t.Fatal(err)
	}
	param := ParamAttr(string(vector))
	meta := OriginTrialMetaTag(string(vector))
	runtime := RuntimeScriptTag(string(vector), string(vector))
	tmpl, err := template.New("page").Parse(`<form {{.Form}}><input {{.Param}}></form>{{.Meta}}{{.Runtime}}`)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := tmpl.Execute(&output, map[string]any{"Form": attrs, "Param": param, "Meta": meta, "Runtime": runtime}); err != nil {
		t.Fatal(err)
	}
	got := output.String()
	if strings.Contains(got, "ZgotmplZ") || strings.Contains(got, "<img") || !strings.Contains(got, html.EscapeString(string(vector))) {
		t.Fatalf("unsafe or unusable helpers: %s", got)
	}
	if !strings.Contains(got, `toolname="create_task"`) || !strings.Contains(got, " toolautosubmit") || strings.Contains(got, "toolparamtitle") {
		t.Fatal("invalid attributes")
	}
	attrs, err = FormAttrs("valid.name-1", "Description", false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(attrs), "toolautosubmit") {
		t.Fatal("false boolean attribute emitted")
	}
	if _, err := FormAttrs("bad name", "x", false); err == nil {
		t.Fatal("invalid form name")
	}
	if _, err := FormAttrs("valid", "", false); err == nil {
		t.Fatal("empty form description")
	}
	if OriginTrialMetaTag("") != "" {
		t.Fatal("empty token tag")
	}
	if RuntimeScriptTag("/runtime.js", "") != `<script type="module" src="/runtime.js"></script>` {
		t.Fatal("unexpected runtime tag")
	}
}

func TestAutostartOptions(t *testing.T) {
	registry := &Registry{}
	manifest, err := registry.Manifest()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		option *bool
		want   bool
	}{{nil, true}, {ptr(true), true}, {ptr(false), false}} {
		tag, err := manifest.ScriptTag(ScriptTagOptions{Autostart: tc.option})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(tag), "data-webmcp-autostart") != tc.want {
			t.Fatalf("autostart: %s", tag)
		}
	}
}
