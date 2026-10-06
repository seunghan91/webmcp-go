package webmcp_test

import (
	"encoding/json"
	"fmt"

	webmcp "github.com/seunghan91/webmcp-go"
)

// Define a read-only tool, select it for a page, and render the manifest.
// The page also needs RuntimeScriptTag pointing at RuntimeHandler.
func Example() {
	tool, err := webmcp.NewTool(webmcp.Def{
		Name:        "list_tasks",
		Description: "List at most 20 tasks for the signed-in user.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"completed": map[string]any{"type": "boolean"},
			},
		},
		Annotations: webmcp.Annotations{"read_only": true, "untrusted_content": true},
		Endpoint:    webmcp.Endpoint{Path: "/api/tasks", Method: "GET"},
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

	manifest, err := registry.Manifest("list_tasks")
	if err != nil {
		panic(err)
	}
	var parsed struct {
		Tools []struct {
			Name        string          `json:"name"`
			Annotations map[string]bool `json:"annotations"`
			Endpoint    struct{ Path, Method string }
		} `json:"tools"`
	}
	raw, _ := json.Marshal(manifest)
	_ = json.Unmarshal(raw, &parsed)
	first := parsed.Tools[0]
	fmt.Println(first.Name, first.Endpoint.Method, first.Endpoint.Path, first.Annotations)
	// Output: list_tasks GET /api/tasks map[readOnlyHint:true untrustedContentHint:true]
}

// A GET endpoint must be read-only; NewTool rejects the definition at boot.
func ExampleNewTool_getRequiresReadOnly() {
	_, err := webmcp.NewTool(webmcp.Def{
		Name:        "delete_task",
		Description: "Delete one task.",
		InputSchema: map[string]any{"type": "object"},
		Endpoint:    webmcp.Endpoint{Path: "/api/tasks/delete", Method: "GET"},
	})
	fmt.Println(err != nil)
	// Output: true
}
