// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eval

import (
	"testing"

	assert "github.com/stretchr/testify/require"
)

func TestParseExpressionError(t *testing.T) {
	assert := assert.New(t)
	expr, err := ParseExpression("~")

	assert.Nil(expr)
	assert.EqualError(err, `1:1: unexpected token "~"`)
}

func TestParseIterableError(t *testing.T) {
	assert := assert.New(t)
	expr, err := ParseIterable("len(5 >)")

	assert.Nil(expr)
	assert.EqualError(err, `1:7: unexpected token ">" (expected ")")`)
}

func TestParsePathError(t *testing.T) {
	assert := assert.New(t)
	expr, err := ParsePath(`=/abc/`)

	assert.Nil(expr)
	assert.EqualError(err, `1:1: unexpected token "="`)
}
