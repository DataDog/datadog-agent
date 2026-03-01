// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

package utils

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValueStringSetter(t *testing.T) {
	var val Value[string]
	setter := ValueStringSetter(&val)

	// successful set
	setter("hello", nil)
	v, err := val.Value()
	require.NoError(t, err)
	assert.Equal(t, "hello", v)

	// error set
	setter("", errors.New("fail"))
	_, err = val.Value()
	assert.Error(t, err)
}

func TestValueParseInt64Setter(t *testing.T) {
	var val Value[uint64]
	setter := ValueParseInt64Setter(&val)

	setter("42", nil)
	v, err := val.Value()
	require.NoError(t, err)
	assert.Equal(t, uint64(42), v)

	// invalid number propagates parse error
	setter("notanumber", nil)
	_, err = val.Value()
	assert.Error(t, err)
}

func TestValueParseFloat64Setter(t *testing.T) {
	var val Value[float64]
	setter := ValueParseFloat64Setter(&val)

	setter("3.14", nil)
	v, err := val.Value()
	require.NoError(t, err)
	assert.InDelta(t, 3.14, v, 0.001)

	// error passed through
	setter("", errors.New("fail"))
	_, err = val.Value()
	assert.Error(t, err)
}
