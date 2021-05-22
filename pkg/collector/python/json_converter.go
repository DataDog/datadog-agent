package python

import (
	"github.com/StackVista/stackstate-agent/pkg/util/log"
	"gopkg.in/yaml.v2"
)
import "C"

// A yaml string is provided from the C bindings in order to pass an arbitrary data structure to Go
// (eg. topology component data or topology event data)
// Here we first unmarshal the string into a map[interface]interface and then covert all
// map keys to string (making a de facto json structure), which will be serialized without problems to json when sent.
func yamlDataToJSON(data *C.char) map[string]interface{} {
	defer recoverFromPanic()

	_data := make(map[interface{}]interface{})
	err := yaml.Unmarshal([]byte(C.GoString(data)), _data)
	if err != nil {
		log.Error(err)
		return nil
	}

	return convertKeysToString(_data).(map[string]interface{})
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

func recoverFromPanic() {
	if r := recover(); r != nil {
		_ = log.Error("Type conversion errors while turning map[interface] to map[string]", r)
	}
}
