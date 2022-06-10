package json

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
