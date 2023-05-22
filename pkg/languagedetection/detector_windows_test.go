// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package languagedetection

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLanguageFromCommandline(t *testing.T) {
	for _, tc := range []struct {
		name     string
		cmdline  []string
		expected LanguageName
		error    bool
	}{
		{
			name:     "windows python",
			cmdline:  []string{"C:\\Program Files\\Python3.9\\python.exe", "test.py"},
			expected: python,
		},
		{
			name:     "java",
			cmdline:  []string{"C:\\Program Files\\Java\\java.exe", "main.java"},
			expected: java,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, languageNameFromCommandLine(tc.cmdline))
		})
	}
}
