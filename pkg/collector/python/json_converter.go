package python

import (
	"github.com/StackVista/stackstate-agent/pkg/util/log"
	"gopkg.in/yaml.v2"
)
import "C"

// A yaml string is provided from the C bindings in order to pass an arbitrary data structure
// (eg. topology component data or topology event data)
// Here we first unmarshal the string into a map[interface]interface and then covert all
// map keys to string (making a de facto json structure), which will be serialized without problems to json when sent.
func yamlDataToJson(data *C.char) map[string]interface{} {
	_data := make(map[interface{}]interface{})
	err := yaml.Unmarshal([]byte(C.GoString(data)), _data)
	if err != nil {
		log.Error(err)
		return nil
	}

	return convertKeysToString(_data)
}

// Recursively cast all the keys of all maps to string
func convertKeysToString(m map[interface{}]interface{}) map[string]interface{} {
	defer recoverFromPanic()

	res := map[string]interface{}{}
	for k, v := range m {
		switch v2 := v.(type) {
		case map[interface{}]interface{}:
			res[k.(string)] = convertKeysToString(v2)
			//res[fmt.Sprint(k)] = convertKeysToString(v2)
		default:
			res[k.(string)] = v
			//res[fmt.Sprint(k)] = v
		}
	}
	return res
}

func recoverFromPanic() {
	if r := recover(); r != nil {
		_ = log.Error("Type conversion errors while turning map[interface] to map[string]", r)
	}
}
