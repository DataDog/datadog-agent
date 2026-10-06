// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestToScreamingSnakeCase(t *testing.T) {
	cases := map[string]string{
		"name":       "NAME",
		"targetURL":  "TARGET_URL",
		"HTTPCode":   "HTTP_CODE",
		"repoName":   "REPO_NAME",
		"repoUrl":    "REPO_URL",
		"a":          "A",
		"":           "",
		"already_ok": "ALREADY_OK",
	}
	for input, expected := range cases {
		assert.Equal(t, expected, ToScreamingSnakeCase(input), "input %q", input)
	}
}
