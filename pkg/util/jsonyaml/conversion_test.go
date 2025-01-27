// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package jsonyaml

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const YAMLDoc = `
int: 42
float: 3.14
string: "hello"
array:
  - 1
  - 2
  - 3
object:
  key: value
  nested:
    - 1
    - 2
    - 3
`

const JSONDoc = `{
	"int": 42,
	"float": 3.14,
	"string": "hello",
	"array": [1, 2, 3],
	"object": {
		"key": "value",
		"nested": [1, 2, 3]
	}
}`

const YAMLDoc2 = `
hello: stringKey
42:    intKey
3.14:  floatKey
true:  boolKey
inner:
  - hello: stringKey
    42:    intKey
    3.14:  floatKey
    true:  boolKey
`

const JSONDoc2 = `{
	"hello": "stringKey",
	"42":    "intKey",
	"3.14":  "floatKey",
	"true":  "boolKey",
	"inner": [
		{
			"hello": "stringKey",
			"42":    "intKey",
			"3.14":  "floatKey",
			"true":  "boolKey"
		}
	]
}`

func TestYAMLToJSON(t *testing.T) {
	tests := []struct {
		name     string
		yamlData string
		jsonData string
	}{
		{
			name:     "simple",
			yamlData: YAMLDoc,
			jsonData: JSONDoc,
		},
		{
			name:     "non-string map keys",
			yamlData: YAMLDoc2,
			jsonData: JSONDoc2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonData, err := YAMLToJSON([]byte(tt.yamlData))
			assert.NoError(t, err)
			assert.JSONEq(t, tt.jsonData, string(jsonData))
		})
	}
}

func TestJSONToYAML(t *testing.T) {
	tests := []struct {
		name     string
		jsonData string
		yamlData string
	}{
		{
			name:     "simple",
			jsonData: JSONDoc,
			yamlData: YAMLDoc,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yamlData, err := JSONToYAML([]byte(JSONDoc))
			assert.NoError(t, err)
			assert.YAMLEq(t, YAMLDoc, string(yamlData))
		})
	}
}
