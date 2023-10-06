// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package languagedetection

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
)

func TestDetectLanguage(t *testing.T) {
	for _, tc := range []struct {
		name     string
		cmdline  []string
		expected languagemodels.LanguageName
		error    bool
	}{
		{
			name:     "Python",
			cmdline:  []string{"C:\\Program Files\\Python3.9\\Python.exe", "test.py"},
			expected: languagemodels.Python,
		},
		{
			name:     "Java",
			cmdline:  []string{"C:\\Program Files\\Java\\Java.exe", "main.Java"},
			expected: languagemodels.Java,
		},
		{
			name:     "ingore javac",
			cmdline:  []string{"C:\\Program Files\\Java\\javac.exe", "main.Java"},
			expected: languagemodels.Unknown,
		},
		{
			name:     "dotnet",
			cmdline:  []string{"dotnet", "BankApp.dll"},
			expected: languagemodels.Dotnet,
		},
		{
			name:     "rubyw",
			cmdline:  []string{"C:\\Users\\AppData\\bin\\rubyw.exe", "prog.rb"},
			expected: languagemodels.Ruby,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			process := []languagemodels.Process{makeProcess(tc.cmdline, "")}
			expected := []*languagemodels.Language{{Name: tc.expected}}
			assert.Equal(t, expected, DetectLanguage(process, nil))
		})
	}
}

func TestGetExe(t *testing.T) {
	type test struct {
		name     string
		cmdline  []string
		expected string
	}
	for _, tc := range []test{
		{
			name:     "windows",
			cmdline:  []string{"C:\\Program Files\\Python\\python.exe", "test.py"},
			expected: "python",
		},
		{
			name:     "quotes",
			cmdline:  []string{"\"C:\\Program Files\\Python\\python.exe\"", "test.py"},
			expected: "python",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, getExe(tc.cmdline))
		})
	}
}
