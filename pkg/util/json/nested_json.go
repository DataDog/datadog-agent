package json

import "strings"

// GetNestedValue returns the value in the map specified by the array keys,
// where each value is another depth level in the map.
// Returns nil if the map doesn't contain the nested key.
func GetNestedValue(inputMap map[string]interface{}, keys ...string) interface{} {
	if _, exists := inputMap[keys[0]]; !exists {
		return nil
	}

	if val, exists := inputMap[keys[0]]; exists {
		if len(keys) == 1 {
			return val
		}
		return GetNestedValue(val.(map[string]interface{}), keys[1:]...)
	}

	return nil
}

func lower(f interface{}) interface{} {
	switch f := f.(type) {
	case []interface{}:
		for i := range f {
			f[i] = lower(f[i])
		}
		return f
	case map[string]interface{}:
		lf := make(map[string]interface{}, len(f))
		for k, v := range f {
			lf[strings.ToLower(k)] = lower(v)
		}
		return lf
	default:
		return f
	}
}
