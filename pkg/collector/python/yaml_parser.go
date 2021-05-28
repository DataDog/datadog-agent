package python

import (
	"github.com/StackVista/stackstate-agent/pkg/util/log"
	"gopkg.in/yaml.v2"
)
import "C"

// A yaml string is provided from the C bindings in order to pass an arbitrary yaml structure to Go
// (eg. topology component yaml or topology event yaml)
// Here we first unmarshal the string into a map[interface]interface and then covert all
// map keys to string (making a de facto json structure), which will be serialized without problems to json when sent.
//
// Note: This function can panic if the yaml has no string keys; reason being it cannot be marshalled to json.
//       We assume the yaml is already validated on the caller (python) side.
func unsafeParseYamlToMap(data *C.char) (map[string]interface{}, error) {
	_data := make(map[interface{}]interface{})
	err := yaml.Unmarshal([]byte(C.GoString(data)), _data)
	if err != nil {
		log.Errorf("Cannot unmarshal yaml: %v", err)
		return nil, err
	}

	return convertKeysToString(_data).(map[string]interface{}), nil
}

// Recursively cast all the keys of all maps to string
func convertKeysToString(i interface{}) interface{} {
	switch x := i.(type) {
	case map[interface{}]interface{}:
		m2 := map[string]interface{}{}
		for k, v := range x {
			m2[k.(string)] = convertKeysToString(v)
		}
		return m2
	case []interface{}:
		for i, v := range x {
			x[i] = convertKeysToString(v)
		}
	}
	return i
}
