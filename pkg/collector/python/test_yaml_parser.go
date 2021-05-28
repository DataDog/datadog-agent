// +build python,test

package python

import (
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
)

import "C"

func testParsingMapWithDifferentTypes(t *testing.T) {
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

	convertedMap, err := unsafeParseYamlToMap(yaml)
	assert.Equal(t, nil, err)

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

	_, err = json.Marshal(convertedMap)
	assert.Equal(t, nil, err)
}

func testParsingInnerMapsWithStringKey(t *testing.T) {
	// we expect all inner maps to be map[string] so they can be serialized to json
	yaml := C.CString(`
yaml:
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

	convertedMap, err := unsafeParseYamlToMap(yaml)
	assert.Equal(t, nil, err)

	expectedJsonMap := map[string]interface{}{
		"yaml": map[string]interface{}{
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

	_, err = json.Marshal(convertedMap)
	assert.Equal(t, nil, err)
}

func testErrorParsingNonMapYaml(t *testing.T) {
	// we expect the conversion of anything not being map[string] to return an error

	tests := []struct {
		name string
		yaml string
	} {
		{
			name: "string instead of map",
			yaml: `I'm such A sentence!`,
		},
		{
			name: "list instead of map",
			yaml: `
  - this
  - is
  - a
  - list`,
		},
		{
			name: "map with array key",
			yaml: `yaml:
                     [a, b, c]: true`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := unsafeParseYamlToMap(C.CString(tt.yaml))
			assert.NotEqual(t, nil, err)
			assert.Equal(t, 0, len(res)) //empty map
		})
	}
}

func recoverParsingError() {
	if r := recover(); r != nil {
		println(fmt.Sprintf("Type conversion errors while turning map[interface] to map[string]: %v", recover()))
	}
}

func testErrorParsingNonStringKeys(t *testing.T) {
	// we expect the conversion of map keys not being strings to panic and return nothing

	tests := []struct {
		name string
		yaml string
	} {
		{
			name: "int key",
			yaml: `yaml:
                     0: true`,
		},
		{
			name: "null key",
			yaml: `yaml:
                     null: true`,
		},

	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer recoverParsingError()

			yaml := C.CString(tt.yaml)
			res, err := unsafeParseYamlToMap(yaml)
			assert.Equal(t, 0, len(res)) //empty map
			assert.Equal(t, nil, err)

			// Never reaches here if `OtherFunctionThatPanics` panics.
			t.Errorf("did not panic")
		})
	}
}
