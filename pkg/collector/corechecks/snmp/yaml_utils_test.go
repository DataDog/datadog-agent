package snmp

import (
	"gopkg.in/yaml.v2"
	"testing"

	"github.com/stretchr/testify/assert"
)

type MyStringArray struct {
	SomeIds StringArray `yaml:"my_field"`
}
type MyNumber struct {
	SomeNum Number `yaml:"my_field"`
}

func TestStringArray_UnmarshalYAML_array(t *testing.T) {
	myStruct := MyStringArray{}
	expected := MyStringArray{SomeIds: StringArray{"aaa", "bbb"}}

	yaml.Unmarshal([]byte(`
my_field:
 - aaa
 - bbb
`), &myStruct)

	assert.Equal(t, expected, myStruct)
}

func TestStringArray_UnmarshalYAML_string(t *testing.T) {
	myStruct := MyStringArray{}
	expected := MyStringArray{SomeIds: StringArray{"aaa"}}

	yaml.Unmarshal([]byte(`
my_field: aaa
`), &myStruct)

	assert.Equal(t, expected, myStruct)
}

func Test_metricTagConfig_UnmarshalYAML(t *testing.T) {
	myStruct := metricsConfig{}
	expected := metricsConfig{MetricTags: []metricTagConfig{{Index: 3}}}

	yaml.Unmarshal([]byte(`
metric_tags:
- index: 3
`), &myStruct)

	assert.Equal(t, expected, myStruct)
}

func Test_metricTagConfig_onlyTags(t *testing.T) {
	myStruct := metricsConfig{}
	expected := metricsConfig{MetricTags: []metricTagConfig{{symbolTag: "aaa"}}}

	yaml.Unmarshal([]byte(`
metric_tags:
- aaa
`), &myStruct)

	assert.Equal(t, expected, myStruct)
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
