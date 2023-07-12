// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tests

import (
	"reflect"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/stretchr/testify/assert"
)

func TestInputSpecsYAMLAndJSONFields(t *testing.T) {
	{
		spec := &compliance.InputSpec{}
		root := reflect.TypeOf(spec).Elem()
		recursiveTagChecks(t, root)
	}
	{
		spec := &compliance.Rule{}
		root := reflect.TypeOf(spec).Elem()
		recursiveTagChecks(t, root)
	}
	{
		spec := &compliance.Benchmark{}
		root := reflect.TypeOf(spec).Elem()
		recursiveTagChecks(t, root)
	}
}

func recursiveTagChecks(t *testing.T, strct reflect.Type) {
	if strct.Kind() == reflect.Map {
		return
	}
	assert.NotZero(t, strct.NumField())
	for i := 0; i < strct.NumField(); i++ {
		field := strct.FieldByIndex([]int{i})
		if field.IsExported() {
			assert.NotEmpty(t, field.Tag.Get("yaml"), "expecting yaml tag for field: %s", field.Name)
			assert.NotEmpty(t, field.Tag.Get("json"), "expecting json tag for field: %s", field.Name)
			assert.Equal(t, field.Tag.Get("yaml"), field.Tag.Get("json"), "expecting json and yaml tag to be equal for field: %s", field.Name)
			if field.Type.Kind() == reflect.Pointer {
				recursiveTagChecks(t, field.Type.Elem())
			} else if field.Type.Kind() == reflect.Struct {
				recursiveTagChecks(t, field.Type)
			}
		}
	}
}
