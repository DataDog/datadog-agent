package python

import (
	"fmt"
	"github.com/StackVista/stackstate-agent/pkg/util/log"
	"gopkg.in/yaml.v2"
)
import "C"

// A yaml string is provided from the C bindings in order to pass an arbitrary yaml structure to Go
// (eg. topology component yaml or topology event yaml)
// Here we first unmarshal the string into a map[interface]interface and then covert all
// map keys to string (making a de facto json structure), which will be serialized without problems to json when sent.
//
func tryParseYamlToMap(data *C.char) (map[string]interface{}, error) {
	_data := make(map[interface{}]interface{})
	err := yaml.Unmarshal([]byte(C.GoString(data)), _data)
	if err != nil {
		log.Errorf("Cannot unmarshal yaml: %v", err)
		return nil, err
	}

	result, err := convertKeysToString(_data)

	if err == nil {
		return result.(map[string]interface{}), nil
	}
	log.Errorf("Got error")
	return nil, err
}

// Recursively cast all the keys of all maps to string
func convertKeysToString(i interface{}) (interface{}, error) {
	switch x := i.(type) {
	case map[interface{}]interface{}:
		m2 := map[string]interface{}{}
		for k, v := range x {
			switch keyString := k.(type) {
			case string:
				value, err := convertKeysToString(v)
				if err == nil {
					m2[keyString] = value
				} else {
					return nil, err
				}
			default:
				return nil, fmt.Errorf("got key other than type string: %T", k)
			}
		}
		return m2, nil
	case []interface{}:
		a2 := make([]interface{}, len(x))
		for i, v := range x {
			value, err := convertKeysToString(v)
			if err == nil {
				a2[i] = value
			} else {
				return nil, err
			}
		}
		return a2, nil
	default:
		return i, nil
	}
}
