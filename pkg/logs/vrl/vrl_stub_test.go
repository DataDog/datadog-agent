// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !vrl

package vrl

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStubCompileFailsClearly(t *testing.T) {
	prog, err := Compile(`.status == "debug"`)
	assert.Nil(t, prog)
	assert.ErrorContains(t, err, "vrl' build tag")
}

func TestStubProgramMethodsDoNotPanic(t *testing.T) {
	var prog *Program

	matched, err := prog.Filter([]byte("hello"))
	assert.False(t, matched)
	assert.Error(t, err)

	out, err := prog.Transform([]byte("hello"))
	assert.Equal(t, []byte("hello"), out)
	assert.Error(t, err)

	assert.NotPanics(t, func() { prog.Close() })
}
