// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build vrl

package vrl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompileInvalidSyntax(t *testing.T) {
	_, err := Compile(".message ==")
	assert.Error(t, err)
}

func TestFilterMatch(t *testing.T) {
	prog, err := Compile(`parse_json!(.message).level == "error"`)
	require.NoError(t, err)
	defer prog.Close()

	matched, err := prog.Filter([]byte(`{"level":"error"}`))
	require.NoError(t, err)
	assert.True(t, matched)

	matched, err = prog.Filter([]byte(`{"level":"debug"}`))
	require.NoError(t, err)
	assert.False(t, matched)
}

func TestFilterRuntimeError(t *testing.T) {
	prog, err := Compile(`parse_json!(.message).level == "error"`)
	require.NoError(t, err)
	defer prog.Close()

	matched, err := prog.Filter([]byte("not json"))
	assert.False(t, matched)
	assert.Error(t, err)
}

func TestFilterEmptyMessage(t *testing.T) {
	prog, err := Compile(`.message == ""`)
	require.NoError(t, err)
	defer prog.Close()

	matched, err := prog.Filter(nil)
	require.NoError(t, err)
	assert.True(t, matched)
}

func TestTransformRedactsMessage(t *testing.T) {
	prog, err := Compile(`.message = redact!(.message, [r'\d+'])`)
	require.NoError(t, err)
	defer prog.Close()

	out, err := prog.Transform([]byte("my id is 123456"))
	require.NoError(t, err)
	assert.Equal(t, []byte("my id is [REDACTED]"), out)
}

func TestTransformErrorReturnsOriginalMessage(t *testing.T) {
	prog, err := Compile(`. = {"other": "field"}`)
	require.NoError(t, err)
	defer prog.Close()

	input := []byte("hello")
	out, err := prog.Transform(input)
	assert.Error(t, err)
	assert.Equal(t, input, out)
}

func TestNilProgram(t *testing.T) {
	var prog *Program

	_, err := prog.Filter([]byte("hello"))
	assert.Error(t, err)

	_, err = prog.Transform([]byte("hello"))
	assert.Error(t, err)

	assert.NotPanics(t, func() { prog.Close() })
}
