// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build linux

package cgroups

import (
	"github.com/stretchr/testify/assert"
	"strings"
	"testing"
)

func TestMunch(t *testing.T) {

	r := []byte("hello world     it's me   the clown🤡 ")
	tok, rest := munchWhitespace(r)
	assert.Equal(t, "hello", string(tok))

	tok, rest = munchWhitespace(rest)
	assert.Equal(t, "world", string(tok))

	tok, rest = munchWhitespace(rest)
	assert.Equal(t, "it's", string(tok))

	tok, rest = munchWhitespace(rest)
	assert.Equal(t, "me", string(tok))

	tok, rest = munchWhitespace(rest)
	assert.Equal(t, "the", string(tok))

	tok, _ = munchWhitespace(rest)
	assert.Equal(t, "clown🤡", string(tok))
}

func TestParseFile(t *testing.T) {

	bigString := &strings.Builder{}
	for line := 0; line < 100; line++ {
		// this string is wide enough to hit the initial buffer size in bufio.Reader
		for char := 0; char < 1000; char++ {
			bigString.WriteByte('X')
		}
		bigString.WriteRune('\n')
	}
	expected := bigString.String()

	actualString := &strings.Builder{}
	readFile(strings.NewReader(expected), func(bytes []byte) error {
		actualString.Write(bytes)
		actualString.WriteRune('\n')
		return nil
	})

	assert.Equal(t, expected, actualString.String())
}
