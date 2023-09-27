// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package profiledefinition

import (
	"testing"

	"gopkg.in/yaml.v2"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type MyStringArray struct {
	SomeIds StringArray `yaml:"my_field"`
}

type MyKeyValueList struct {
	KvListField KeyValueList `yaml:"my_kv_list"`
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

func TestKeyValueList_UnmarshalYAML_mapping(t *testing.T) {
	myStruct := MyKeyValueList{}
	expected := MyKeyValueList{KvListField: KeyValueList{
		{
			Key: "1", Value: "aaa",
		},
		{
			Key: "2", Value: "bbb",
		},
	}}

	err := yaml.Unmarshal([]byte(`
my_kv_list:
 1: aaa
 2: bbb
`), &myStruct)
	assert.NoError(t, err)

	assert.Equal(t, expected, myStruct)
}

func TestKeyValueList_UnmarshalYAML_listKeyValue_listNotAllowedInYaml(t *testing.T) {
	myStruct := MyKeyValueList{}

	err := yaml.Unmarshal([]byte(`
my_kv_list:
 - key: 1
   value: aaa
 - key: 2
   value: bbb
`), &myStruct)
	assert.ErrorContains(t, err, "cannot unmarshal !!seq into map[string]string")
}

func TestKeyValueList_Marshall(t *testing.T) {
	myStruct := MyKeyValueList{
		KvListField: KeyValueList{
			{Key: "1", Value: "aaa"},
			{Key: "2", Value: "bbb"},
		},
	}

	bytes, err := yaml.Marshal(myStruct)
	require.NoError(t, err)
	expectedYaml := `my_kv_list:
  "1": aaa
  "2": bbb
`
	assert.Equal(t, expectedYaml, string(bytes))
}
