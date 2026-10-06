package webmcp

import (
	"fmt"
	"html"
	"html/template"
	"unicode/utf8"
)

func nonceAttr(nonce string) string {
	if nonce == "" {
		return ""
	}
	return ` nonce="` + html.EscapeString(nonce) + `"`
}

// RuntimeScriptTag loads an external runtime module from the application's URL.
// src and nonce are escaped independently, including values cast from safe HTML.
func RuntimeScriptTag(src, nonce string) template.HTML {
	return template.HTML(`<script type="module" src="` + html.EscapeString(src) + `"` + nonceAttr(nonce) + `></script>`)
}

// OriginTrialMetaTag renders the token, or nothing for an empty token.
func OriginTrialMetaTag(token string) template.HTML {
	if token == "" {
		return ""
	}
	return template.HTML(`<meta http-equiv="origin-trial" content="` + html.EscapeString(token) + `">`)
}

// FormAttrs emits only the declarative form attributes defined by WebMCP.
// Descriptions must be trusted application metadata, not visitor-controlled text.
func FormAttrs(tool, description string, autosubmit bool) (template.HTMLAttr, error) {
	if err := validateName(tool); err != nil {
		return "", err
	}
	if description == "" || !utf8.ValidString(description) {
		return "", fmt.Errorf("form description must be a nonempty UTF-8 string")
	}
	attrs := `toolname="` + html.EscapeString(tool) + `" tooldescription="` + html.EscapeString(description) + `"`
	if autosubmit {
		attrs += ` toolautosubmit`
	}
	return template.HTMLAttr(attrs), nil
}

// ParamAttr renders escaped field metadata for use in html/template.
func ParamAttr(description string) template.HTMLAttr {
	return template.HTMLAttr(`toolparamdescription="` + html.EscapeString(description) + `"`)
}
