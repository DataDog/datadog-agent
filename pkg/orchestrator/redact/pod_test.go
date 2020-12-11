// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package redact

import (
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
)

func TestScrubContainer(t *testing.T) {
	scrubber := NewDefaultDataScrubber()
	tests := getScrubCases()
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ScrubContainer(&tc.input, scrubber)
			assert.Equal(t, tc.expected, tc.input)
		})
	}
}

func getScrubCases() map[string]struct {
	input    v1.Container
	expected v1.Container
} {
	tests := map[string]struct {
		input    v1.Container
		expected v1.Container
	}{
		"sensitive CLI": {
			input: v1.Container{
				Command: []string{"mysql", "--password", "afztyerbzio1234"},
			},
			expected: v1.Container{
				Command: []string{"mysql", "--password", "********"},
			},
		},
		"empty container": {
			input:    v1.Container{},
			expected: v1.Container{},
		},
		"empty container empty slices": {
			input: v1.Container{
				Command: []string{},
				Args:    []string{},
			},
			expected: v1.Container{
				Command: []string{},
				Args:    []string{},
			},
		},
		"non sensitive CLI": {
			input: v1.Container{
				Command: []string{"mysql", "--arg", "afztyerbzio1234"},
			},
			expected: v1.Container{
				Command: []string{"mysql", "--arg", "afztyerbzio1234"},
			},
		},
		"non sensitive CLI joined": {
			input: v1.Container{
				Command: []string{"mysql --arg afztyerbzio1234"},
			},
			expected: v1.Container{
				Command: []string{"mysql --arg afztyerbzio1234"},
			},
		},
		"sensitive CLI joined": {
			input: v1.Container{
				Command: []string{"mysql --password afztyerbzio1234"},
			},
			expected: v1.Container{
				Command: []string{"mysql", "--password", "********"},
			},
		},
		"sensitive env var": {
			input: v1.Container{
				Env: []v1.EnvVar{{Name: "password", Value: "kqhkiG9w0BAQEFAASCAl8wggJbAgEAAoGBAOLJ"}},
			},
			expected: v1.Container{
				Env: []v1.EnvVar{{Name: "password", Value: "********"}},
			},
		},
		"command with sensitive arg": {
			input: v1.Container{
				Command: []string{"mysql"},
				Args:    []string{"--password", "afztyerbzio1234"},
			},
			expected: v1.Container{
				Command: []string{"mysql"},
				Args:    []string{"--password", "********"},
			},
		},
		"command with no sensitive arg": {
			input: v1.Container{
				Command: []string{"mysql"},
				Args:    []string{"--debug", "afztyerbzio1234"},
			},
			expected: v1.Container{
				Command: []string{"mysql"},
				Args:    []string{"--debug", "afztyerbzio1234"},
			},
		},
		"sensitive command with no sensitive arg": {
			input: v1.Container{
				Command: []string{"mysql --password 123"},
				Args:    []string{"--debug", "123"},
			},
			expected: v1.Container{
				Command: []string{"mysql", "--password", "********"},
				Args:    []string{"--debug", "123"},
			},
		},
		"sensitive command with no sensitive arg joined": {
			input: v1.Container{
				Command: []string{"mysql --password 123"},
				Args:    []string{"--debug 123"},
			},
			expected: v1.Container{
				Command: []string{"mysql", "--password", "********"},
				Args:    []string{"--debug", "123"},
			},
		},
		"sensitive command with no sensitive arg split": {
			input: v1.Container{
				Command: []string{"mysql", "--password", "123"},
				Args:    []string{"--debug", "123"},
			},
			expected: v1.Container{
				Command: []string{"mysql", "--password", "********"},
				Args:    []string{"--debug", "123"},
			},
		},
		"sensitive command with no sensitive arg mixed": {
			input: v1.Container{
				Command: []string{"mysql", "--password", "123"},
				Args:    []string{"--debug", "123"},
			},
			expected: v1.Container{
				Command: []string{"mysql", "--password", "********"},
				Args:    []string{"--debug", "123"},
			},
		},
		"sensitive pass in args": {
			input: v1.Container{
				Command: []string{"agent --password"},
				Args:    []string{"token123"},
			},
			expected: v1.Container{
				Command: []string{"agent", "--password"},
				Args:    []string{"********"},
			},
		},
		"sensitive arg no command": {
			input: v1.Container{
				Args: []string{"password", "--password", "afztyerbzio1234"},
			},
			expected: v1.Container{
				Args: []string{"password", "--password", "********"},
			},
		},
		"sensitive arg joined": {
			input: v1.Container{
				Args: []string{"pwd pwd afztyerbzio1234 --password 1234"},
			},
			expected: v1.Container{
				Args: []string{"pwd", "pwd", "********", "--password", "********"},
			},
		},
		"sensitive container": {
			input: v1.Container{
				Name:    "test container",
				Image:   "random",
				Command: []string{"decrypt", "--password", "afztyerbzio1234", "--access_token", "yolo123"},
				Env: []v1.EnvVar{
					{Name: "hostname", Value: "password"},
					{Name: "pwd", Value: "yolo"},
				},
				Args: []string{"--password", "afztyerbzio1234"},
			},
			expected: v1.Container{
				Name:    "test container",
				Image:   "random",
				Command: []string{"decrypt", "--password", "********", "--access_token", "********"},
				Env: []v1.EnvVar{
					{Name: "hostname", Value: "password"},
					{Name: "pwd", Value: "********"},
				},
				Args: []string{"--password", "********"},
			},
		},
	}
	return tests
}
