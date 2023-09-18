// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package profiledefinition

import (
	"testing"

	"gopkg.in/yaml.v2"

	"github.com/stretchr/testify/assert"
)

type MyStringArray struct {
	SomeIds StringArray `yaml:"my_field"`
}

type MyMapping struct {
	AMapping MappingArray `yaml:"my_mapping"`
}

func Test_metricTagConfig_UnmarshalYAML(t *testing.T) {
	myStruct := MetricsConfig{}
	expected := MetricsConfig{MetricTags: []MetricTagConfig{{Index: 3}}}

	yaml.Unmarshal([]byte(`
metric_tags:
- index: 3
`), &myStruct)

	assert.Equal(t, expected, myStruct)
}

func Test_metricTagConfig_onlyTags(t *testing.T) {
	myStruct := MetricsConfig{}
	expected := MetricsConfig{MetricTags: []MetricTagConfig{{SymbolTag: "aaa"}}}

	yaml.Unmarshal([]byte(`
metric_tags:
- aaa
`), &myStruct)

	assert.Equal(t, expected, myStruct)
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

func TestMappingArray_UnmarshalYAML_KeyValueArray(t *testing.T) {
	myStruct := MyMapping{}
	expected := MyMapping{
		AMapping: MappingArray{
			KeyValue{Key: "aaa", Value: "111"},
			KeyValue{Key: "bbb", Value: "222"},
		},
	}

	err := yaml.Unmarshal([]byte(`
my_mapping:
 - key: aaa
   value: 111
 - key: bbb
   value: 222
`), &myStruct)
	assert.NoError(t, err)

	assert.Equal(t, expected, myStruct)
}

func TestMappingArray_UnmarshalYAML_KeyValueArray_ErrorSameKeyUsedMultipleTime(t *testing.T) {
	myStruct := MyMapping{}

	err := yaml.Unmarshal([]byte(`
my_mapping:
 - key: aaa
   value: 111
 - key: bbb
   value: 222
 - key: bbb
   value: 333
`), &myStruct)
	assert.Error(t, err, "same key used multiple times: bbb")
}

func TestMappingArray_UnmarshalYAML_StringToStringMapping(t *testing.T) {
	myStruct := MyMapping{}
	expected := MyMapping{
		AMapping: MappingArray{
			KeyValue{Key: "aaa", Value: "111"},
			KeyValue{Key: "bbb", Value: "222"},
		},
	}

	yaml.Unmarshal([]byte(`
my_mapping:
 aaa: 111
 bbb: 222
`), &myStruct)

	assert.Equal(t, expected, myStruct)
}
