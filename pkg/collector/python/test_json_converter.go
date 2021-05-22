// +build python,test

package python

import (
	"encoding/json"
	"github.com/stretchr/testify/assert"
	"testing"
)

import "C"

func testConvertingMapWithDifferentTypes(t *testing.T) {
	yaml := C.CString(`
key: value ®
stringlist: 
  - a
  - b
  - c
boollist:
  - true
  - false
intlist:
  - 1
doublelist:
  - 0.7
  - 1.42
emptykey: null
nestedobject:
  nestedkey: nestedValue
  animals:
    legs: dog
    wings: eagle
    tail: crocodile`)

	convertedMap := yamlDataToJSON(yaml)

	expectedJsonMap := map[string]interface{}{
		"key":        "value ®",
		"stringlist": []interface{}{"a", "b", "c"},
		"boollist":   []interface{}{true, false},
		"intlist":    []interface{}{1},
		"doublelist": []interface{}{0.7, 1.42},
		"emptykey":   nil,
		"nestedobject": map[string]interface{}{
			"nestedkey": "nestedValue",
			"animals": map[string]interface{}{
				"legs":  "dog",
				"wings": "eagle",
				"tail":  "crocodile",
			},
		},
	}

	assert.Equal(t, expectedJsonMap, convertedMap)

	_, err := json.Marshal(convertedMap)
	assert.Equal(t, nil, err)
}

func testConvertingInnerMapsWithStringKey(t *testing.T) {
	// we expect all inner maps to be map[string] so they can be serialized to json
	yaml := C.CString(`
data:
  checks:
  - is_service_check_health_check: true
    name: Integration Health
    stream_id: -1
  name: stubbed.hostname:agent-integration
  service_checks:
  - conditions:
    - key: host
      value: stubbed.hostname
    - key: tags.integration-type
      value: agent-integration
    name: Service Checks
    stream_id: -1`)

    convertedMap := yamlDataToJSON(yaml)

	expectedJsonMap := map[string]interface{}{
		"data": map[string]interface{}{
			"checks": []interface{}{
				map[string]interface{}{
					"is_service_check_health_check": true,
					"name":                          "Integration Health",
					"stream_id":                     -1,
				},
			},
			"name": "stubbed.hostname:agent-integration",
			"service_checks": []interface{}{
				map[string]interface{}{
					"conditions": []interface{}{
						map[string]interface{}{
							"key":   "host",
							"value": "stubbed.hostname",
						},
						map[string]interface{}{
							"key":   "tags.integration-type",
							"value": "agent-integration",
						},
					},
					"name":      "Service Checks",
					"stream_id": -1,
				},
			},
		},
	}

	assert.Equal(t, expectedJsonMap, convertedMap)

	_, err := json.Marshal(convertedMap)
	assert.Equal(t, nil, err)
}

func testConvertingNonMapYaml(t *testing.T) {
	// we expect the conversion of anything not being map[string] to return nothing
	yamlString := C.CString(`I'm such A sentence!`)
	res := yamlDataToJSON(yamlString)
	assert.Equal(t, 0, len(res)) //empty map

	yamlList := C.CString(`
  - this
  - is
  - a
  - list
`)
	res = yamlDataToJSON(yamlList)
	assert.Equal(t, 0, len(res)) //empty map
}

func testConvertingNonStringKeysYaml(t *testing.T) {
	// we expect the conversion of map with keys not being strings to not panic and return nothing
	yamlIntKey := C.CString(`
      data:
        0: true`)
	res := yamlDataToJSON(yamlIntKey)
	assert.Equal(t, 0, len(res)) //empty map

	yamlNullKey := C.CString(`
      data:
        null: true`)
	res = yamlDataToJSON(yamlNullKey)
	assert.Equal(t, 0, len(res)) //empty map

	yamlArrayKey := C.CString(`
      data:
        [a, b, c]: true`)
	res = yamlDataToJSON(yamlArrayKey)
	assert.Equal(t, 0, len(res)) //empty map
}
