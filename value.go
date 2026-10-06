package webmcp

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"unicode/utf8"
)

const maxInteger int64 = 1 << 53

type visit struct {
	kind    reflect.Kind
	pointer uintptr
}

// normalize copies metadata and rejects values outside the portable JSON subset.
// Named strings (including template.HTML) are copied as ordinary strings.
func normalize(value any) (any, error) {
	return normalizeValue(reflect.ValueOf(value), make(map[visit]bool))
}

func normalizeValue(v reflect.Value, ancestors map[visit]bool) (any, error) {
	if !v.IsValid() {
		return nil, nil
	}
	if v.Kind() == reflect.Interface {
		if v.IsNil() {
			return nil, nil
		}
		return normalizeValue(v.Elem(), ancestors)
	}
	if v.Type() == reflect.TypeOf(json.Number("")) {
		n, err := strconv.ParseInt(v.String(), 10, 64)
		if err != nil || n < -maxInteger || n > maxInteger {
			return nil, fmt.Errorf("metadata numbers must be integers within +/-2^53")
		}
		return n, nil
	}
	switch v.Kind() {
	case reflect.String:
		if !utf8.ValidString(v.String()) {
			return nil, fmt.Errorf("metadata must be valid UTF-8")
		}
		return v.String(), nil
	case reflect.Bool:
		return v.Bool(), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n := v.Int()
		if n < -maxInteger || n > maxInteger {
			return nil, fmt.Errorf("metadata integers must be within +/-2^53")
		}
		return n, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n := v.Uint()
		if n > uint64(maxInteger) {
			return nil, fmt.Errorf("metadata integers must be within +/-2^53")
		}
		return int64(n), nil
	case reflect.Map, reflect.Slice, reflect.Array:
		if v.Kind() != reflect.Array {
			if v.IsNil() {
				return nil, nil
			}
			var ptr uintptr
			if v.Kind() == reflect.Map {
				ptr = uintptr(v.UnsafePointer())
			} else {
				ptr = v.Pointer()
			}
			id := visit{v.Kind(), ptr}
			if ancestors[id] {
				return nil, fmt.Errorf("metadata cannot contain circular references")
			}
			ancestors[id] = true
			defer delete(ancestors, id)
		}
		if v.Kind() == reflect.Map {
			if v.Type().Key().Kind() != reflect.String {
				return nil, fmt.Errorf("metadata keys must be strings")
			}
			result := map[string]any{}
			iter := v.MapRange()
			for iter.Next() {
				key := iter.Key().String()
				if !utf8.ValidString(key) {
					return nil, fmt.Errorf("metadata keys must be valid UTF-8")
				}
				item, err := normalizeValue(iter.Value(), ancestors)
				if err != nil {
					return nil, err
				}
				result[key] = item
			}
			return result, nil
		}
		result := make([]any, v.Len())
		for i := range result {
			item, err := normalizeValue(v.Index(i), ancestors)
			if err != nil {
				return nil, err
			}
			result[i] = item
		}
		return result, nil
	default:
		return nil, fmt.Errorf("metadata must contain JSON values; floats and type %s are not supported", v.Type())
	}
}

func objectKeys(value any, allowed []string, label string) (map[string]any, error) {
	obj, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an object", label)
	}
	for key := range obj {
		found := false
		for _, a := range allowed {
			if a == key {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("unsupported %s key: %s", label, key)
		}
	}
	return obj, nil
}

// canonicalJSON uses maps, so encoding/json recursively sorts every object's
// keys. Go escapes U+2028/U+2029 even with EscapeHTML disabled; decode those two
// escapes without changing literal backslash-u sequences in user strings.
func canonicalJSON(value map[string]any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(value); err != nil {
		return nil, err
	}
	data := bytes.TrimSuffix(buf.Bytes(), []byte("\n"))
	result := make([]byte, 0, len(data))
	for i := 0; i < len(data); i++ {
		if data[i] == '\\' && i+1 < len(data) {
			if i+6 <= len(data) && (string(data[i:i+6]) == `\u2028` || string(data[i:i+6]) == `\u2029`) {
				if data[i+5] == '8' {
					result = append(result, "\u2028"...)
				} else {
					result = append(result, "\u2029"...)
				}
				i += 5
				continue
			}
			result = append(result, data[i], data[i+1])
			i++
			continue
		}
		result = append(result, data[i])
	}
	return result, nil
}

func fingerprint(value map[string]any) (string, error) {
	data, err := canonicalJSON(value)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("sha256:%x", sha256.Sum256(data)), nil
}
