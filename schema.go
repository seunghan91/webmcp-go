package webmcp

import "fmt"

func reserved(name string) bool {
	return name == "__proto__" || name == "constructor" || name == "prototype"
}

func validateSchema(value any) (map[string]any, error) {
	schema, err := objectKeys(value, []string{"type", "properties", "required", "description"}, "input_schema")
	if err != nil {
		return nil, err
	}
	if schema["type"] != "object" {
		return nil, fmt.Errorf("input_schema type must be object")
	}
	properties := map[string]any{}
	if raw, exists := schema["properties"]; exists {
		var ok bool
		properties, ok = raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("properties must be an object")
		}
	}
	for name, property := range properties {
		if reserved(name) {
			return nil, fmt.Errorf("reserved property name: %s", name)
		}
		if err := validateProperty(property, name, false); err != nil {
			return nil, err
		}
	}
	if description, exists := schema["description"]; exists {
		if _, ok := description.(string); !ok {
			return nil, fmt.Errorf("schema description must be a string")
		}
	}
	if raw, exists := schema["required"]; exists {
		required, ok := raw.([]any)
		if !ok {
			return nil, fmt.Errorf("required must contain unique declared property names")
		}
		seen := map[string]bool{}
		for _, item := range required {
			key, ok := item.(string)
			if _, exists := properties[key]; !ok || !exists || seen[key] {
				return nil, fmt.Errorf("required must contain unique declared property names")
			}
			seen[key] = true
		}
	}
	return schema, nil
}

func validateProperty(value any, label string, scalarOnly bool) error {
	allowed := []string{"type", "enum", "description", "default", "minimum", "maximum", "maxLength", "maxItems"}
	if !scalarOnly {
		allowed = append(allowed, "items")
	}
	property, err := objectKeys(value, allowed, "property "+label)
	if err != nil {
		return err
	}
	switch property["type"] {
	case "string", "number", "integer", "boolean":
		if _, exists := property["items"]; exists {
			return fmt.Errorf("%s: items requires array type", label)
		}
	case "array":
		if scalarOnly {
			return fmt.Errorf("%s: nested arrays are not supported", label)
		}
		if err := validateProperty(property["items"], label+".items", true); err != nil {
			return err
		}
	default:
		return fmt.Errorf("%s: only scalar properties and arrays of scalars are supported", label)
	}
	if description, exists := property["description"]; exists {
		if _, ok := description.(string); !ok {
			return fmt.Errorf("%s: description must be a string", label)
		}
	}
	for _, key := range []string{"minimum", "maximum", "maxLength", "maxItems"} {
		if raw, exists := property[key]; exists {
			n, ok := raw.(int64)
			if !ok || ((key == "maxLength" || key == "maxItems") && n < 0) {
				return fmt.Errorf("%s: %s must be an integer (lengths must be nonnegative)", label, key)
			}
		}
	}
	if raw, exists := property["enum"]; exists {
		values, ok := raw.([]any)
		if !ok || len(values) == 0 {
			return fmt.Errorf("%s: enum must be a nonempty array matching the property type", label)
		}
		for _, value := range values {
			if !matches(value, property) {
				return fmt.Errorf("%s: enum must match the property type", label)
			}
		}
	}
	if value, exists := property["default"]; exists && !matches(value, property) {
		return fmt.Errorf("%s: default must match the property type", label)
	}
	return nil
}

func matches(value any, property map[string]any) bool {
	switch property["type"] {
	case "string":
		_, ok := value.(string)
		return ok
	case "number", "integer":
		_, ok := value.(int64)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "array":
		values, ok := value.([]any)
		if !ok {
			return false
		}
		for _, item := range values {
			if !matches(item, property["items"].(map[string]any)) {
				return false
			}
		}
		return true
	}
	return false
}
