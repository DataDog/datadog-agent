// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package util

import "testing"

func TestSanitizeAgentID(t *testing.T) {
	tests := []struct {
		name     string
		agentID  string
		expected string
	}{
		{
			name:     "empty",
			agentID:  "",
			expected: "",
		},
		{
			name:     "no special characters",
			agentID:  "agentID",
			expected: "agentid",
		},
		{
			name:     "with special characters",
			agentID:  "agentID@123",
			expected: "agentid_123",
		},
		{
			name:     "with spaces",
			agentID:  "agent ID",
			expected: "agent_id",
		},
		{
			name:     "with special characters and spaces",
			agentID:  "agent ID@123",
			expected: "agent_id_123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := SanitizeAgentID(tt.agentID)
			if actual != tt.expected {
				t.Errorf("expected: %s, got: %s", tt.expected, actual)
			}
		})
	}
}

func TestSanitizeFileName(t *testing.T) {
	tests := []struct {
		name     string
		fileName string
		expected string
	}{
		{
			name:     "empty",
			fileName: "",
			expected: "",
		},
		{
			name:     "with spaces",
			fileName: "my file.log",
			expected: "my_file.log",
		},
		{
			name:     "with special characters",
			fileName: "fileName@123",
			expected: "fileName_123",
		},
		{
			name:     "with special characters and spaces",
			fileName: "file Name@123",
			expected: "file_Name_123",
		},
		{
			name:     "with special characters and spaces and long name",
			fileName: "in-west-philadelphia-born-and-raised-on-the-playground-was-where-i-spent-most-of-my-days-chillin-out-maxin-relaxin-all-cool-and-all-shootin-some-b-ball-outside-of-the-school-when-a-couple-of-guys-who-were-up-to-no-good-started-making-trouble-in-my-neighborhood-i-got-in-one-little-fight-and-my-mom-got-scared-she-said-youre-movin-with-your-auntie-and-uncle-in-bel-air",
			expected: "in-west-philadelphia-born-and-raised-on-the-playground-was-where-i-spent-most-of-my-days-chillin-out-maxin-relaxin-all-cool-and-all-shootin-some-b-ball-outside-of-the-school-when-a-couple-of-guys-who-were-up-to-no-good-started-making-trouble-in-my-neighbo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := SanitizeFileName(tt.fileName)
			if actual != tt.expected {
				t.Errorf("expected: %s, got: %s", tt.expected, actual)
			}
		})
	}
}
