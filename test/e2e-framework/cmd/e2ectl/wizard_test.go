// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"bufio"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTestRunSuffix(t *testing.T) {
	assert.Equal(t, "", testRunSuffix(""))
	assert.Equal(t, " -run TestFoo", testRunSuffix("TestFoo"))
}

func TestConfirmDestroy(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  bool
	}{
		{"exact match", "kind-nopulumi\n", true},
		{"mismatch", "wrong-name\n", false},
		{"whitespace trimmed", "  kind-nopulumi  \n", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			scanner := bufio.NewScanner(strings.NewReader(tc.input))
			got, err := confirmDestroy(scanner, "kind-nopulumi")
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestConfirmDestroyNoInput(t *testing.T) {
	scanner := bufio.NewScanner(strings.NewReader(""))
	got, err := confirmDestroy(scanner, "kind-nopulumi")
	require.NoError(t, err)
	assert.False(t, got)
}
