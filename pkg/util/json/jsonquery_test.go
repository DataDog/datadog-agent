// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019-2020 Datadog, Inc.

package json

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/itchyny/gojq"
	"github.com/stretchr/testify/assert"
)

func TestJsonQueryParse(t *testing.T) {
	var code *gojq.Code
	var err error

	code, err = Parse(".spec.foo")
	assert.NotNil(t, code)
	assert.NoError(t, err)
	value, found := cache.Cache.Get(jqCachePrefix + ".spec.foo")
	assert.True(t, found)
	assert.Equal(t, code, value)

	code, err = Parse(".$spec.foo")
	assert.Nil(t, code)
	assert.Error(t, err)
}

func TestJsonQueryRun(t *testing.T) {
	object := map[string]interface{}{
		"foo": "bar",
		"baz": []interface{}{"toto", "titi"},
	}

	value, hasValue, err := RunSingleOutput(".foo", object)
	assert.Equal(t, "bar", value)
	assert.True(t, hasValue)
	assert.Nil(t, err)

	value, hasValue, err = RunSingleOutput(".bar", object)
	assert.Equal(t, "", value)
	assert.False(t, hasValue)
	assert.Nil(t, err)

	value, hasValue, err = RunSingleOutput(".%bar", object)
	assert.Equal(t, "", value)
	assert.False(t, hasValue)
	assert.Error(t, err)
}
