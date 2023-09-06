// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cprofstruct

import (
	"testing"

	"gopkg.in/yaml.v2"

	"github.com/stretchr/testify/assert"
)

type MyStringArray struct {
	SomeIds StringArray `yaml:"my_field"`
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
