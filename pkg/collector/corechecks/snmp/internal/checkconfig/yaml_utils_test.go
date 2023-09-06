// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checkconfig

import (
	"testing"

	"gopkg.in/yaml.v2"

	"github.com/stretchr/testify/assert"
)

type MyNumber struct {
	SomeNum Number `yaml:"my_field"`
}

type MyBoolean struct {
	SomeBool Boolean `yaml:"my_field"`
}

type MyInterfaceConfigs struct {
	SomeInterfaceConfigs InterfaceConfigs `yaml:"interface_configs"`
}

func Test_Number_UnmarshalYAML(t *testing.T) {
	tests := []struct {
		name   string
		data   []byte
		result MyNumber
	}{
		{
			name: "integer number",
			data: []byte(`
my_field: 99
`),
			result: MyNumber{SomeNum: 99},
		},
		{
			name: "string number",
			data: []byte(`
my_field: "88"
`),
			result: MyNumber{SomeNum: 88},
		},
		{
			name: "empty string",
			data: []byte(`
my_field: ""
`),
			result: MyNumber{SomeNum: 0},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			myStruct := MyNumber{}
			yaml.Unmarshal(tt.data, &myStruct)
			assert.Equal(t, tt.result, myStruct)
		})
	}
}

func Test_Boolean_UnmarshalYAML(t *testing.T) {
	tests := []struct {
		name   string
		data   []byte
		result MyBoolean
	}{
		{
			name: "boolean true",
			data: []byte(`
my_field: true
`),
			result: MyBoolean{SomeBool: true},
		},
		{
			name: "string boolean true",
			data: []byte(`
my_field: "true"
`),
			result: MyBoolean{SomeBool: true},
		},
		{
			name: "boolean false",
			data: []byte(`
my_field: false
`),
			result: MyBoolean{SomeBool: false},
		},
		{
			name: "string boolean false",
			data: []byte(`
my_field: "false"
`),
			result: MyBoolean{SomeBool: false},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			myStruct := MyBoolean{}
			yaml.Unmarshal(tt.data, &myStruct)
			assert.Equal(t, tt.result, myStruct)
		})
	}
}

func Test_Boolean_UnmarshalYAML_invalid(t *testing.T) {
	myStruct := MyBoolean{}
	data := []byte(`
my_field: "foo"
`)
	err := yaml.Unmarshal(data, &myStruct)
	assert.EqualError(t, err, "cannot convert `foo` to boolean")
}

func Test_InterfaceConfigs_UnmarshalYAML(t *testing.T) {
	tests := []struct {
		name          string
		data          []byte
		result        MyInterfaceConfigs
		expectedError string
	}{
		{
			name: "empty interface config",
			data: []byte(`
interface_configs: ""
`),
			result: MyInterfaceConfigs{},
		},
		{
			name: "interface config as yaml struct",
			data: []byte(`
interface_configs:
  - match_field: "name"
    match_value: "eth0"
    in_speed: 25
    out_speed: 10
`),
			result: MyInterfaceConfigs{
				SomeInterfaceConfigs: InterfaceConfigs{
					{
						MatchField: "name",
						MatchValue: "eth0",
						InSpeed:    25,
						OutSpeed:   10,
					},
				},
			},
		},
		{
			name: "interface config as json string",
			data: []byte(`
interface_configs: '[{"match_field":"name","match_value":"eth0","in_speed":25,"out_speed":10}]'
`),
			result: MyInterfaceConfigs{
				SomeInterfaceConfigs: InterfaceConfigs{
					{
						MatchField: "name",
						MatchValue: "eth0",
						InSpeed:    25,
						OutSpeed:   10,
					},
				},
			},
		},
		{
			name: "invalid json",
			data: []byte(`
interface_configs: '['
`),
			result:        MyInterfaceConfigs{},
			expectedError: "cannot unmarshall json to []snmpintegration.InterfaceConfig: unexpected end of JSON input",
		},
		{
			name: "invalid overall yaml",
			data: []byte(`
interface_configs: {
`),
			result:        MyInterfaceConfigs{},
			expectedError: "yaml: line 2: did not find expected node content",
		},
		{
			name: "invalid interface_configs yaml",
			data: []byte(`
interface_configs: {}
`),
			result:        MyInterfaceConfigs{},
			expectedError: "cannot unmarshall to string: yaml: unmarshal errors",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			myStruct := MyInterfaceConfigs{}
			err := yaml.Unmarshal(tt.data, &myStruct)
			assert.Equal(t, tt.result, myStruct)
			if tt.expectedError != "" {
				assert.ErrorContains(t, err, tt.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
