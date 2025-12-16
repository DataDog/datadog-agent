// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDrainTokenize(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{input: "", expected: []string{}},
		{input: ".", expected: []string{"."}},
		{input: "Hello", expected: []string{"Hello"}},
		{input: " Hello   world ", expected: []string{"Hello", "world"}},
		{input: " Hello  , world ", expected: []string{"Hello", ",", "world"}},
		{input: " Hello.  , world ", expected: []string{"Hello.", ",", "world"}},
		{input: " Hello .  , world ", expected: []string{"Hello", ".", ",", "world"}},
		{input: " Hello ., world ", expected: []string{"Hello", ".", ",", "world"}},
		{input: ".. Hello ., world .. .", expected: []string{".", ".", "Hello", ".", ",", "world", ".", ".", "."}},
		{input: "Hello world", expected: []string{"Hello", "world"}},
		{input: "Hello, world! Hello, world!", expected: []string{"Hello,", "world!", "Hello,", "world!"}},
	}
	for _, test := range tests {
		result := DrainTokenize([]byte(test.input))
		assert.Equal(t, test.expected, result)
	}
}
