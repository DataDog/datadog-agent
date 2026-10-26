// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package containers

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAssertTags(t *testing.T) {
	tests := []struct {
		name                 string
		actualTags           []string
		expectedTags         []*regexp.Regexp
		optionalTags         []*regexp.Regexp
		acceptUnexpectedTags bool
		expectedOutput       string
	}{
		{
			name: "All good",
			actualTags: []string{
				"foo:foo",
				"bar:bar",
				"baz:baz",
			},
			expectedTags: []*regexp.Regexp{
				regexp.MustCompile(`^foo:foo$`),
				regexp.MustCompile(`^bar:`),
				regexp.MustCompile(`:baz$`),
			},
			expectedOutput: "",
		},
		{
			name: "Unexpected tag",
			actualTags: []string{
				"foo:foo",
				"bar:bar",
				"baz:baz",
				"qux:qux",
			},
			expectedTags: []*regexp.Regexp{
				regexp.MustCompile(`^foo:foo$`),
				regexp.MustCompile(`^bar:`),
				regexp.MustCompile(`:baz$`),
			},
			expectedOutput: "unexpected tags: qux:qux",
		},
		{
			name: "Accept unexpected tag",
			actualTags: []string{
				"foo:foo",
				"bar:bar",
				"baz:baz",
				"qux:qux",
			},
			expectedTags: []*regexp.Regexp{
				regexp.MustCompile(`^foo:foo$`),
				regexp.MustCompile(`^bar:`),
				regexp.MustCompile(`:baz$`),
			},
			acceptUnexpectedTags: true,
			expectedOutput:       "",
		},
		{
			name: "Missing tag",
			actualTags: []string{
				"foo:foo",
				"bar:bar",
				"baz:baz",
			},
			expectedTags: []*regexp.Regexp{
				regexp.MustCompile(`^foo:foo$`),
				regexp.MustCompile(`^bar:`),
				regexp.MustCompile(`:baz$`),
				regexp.MustCompile(`^qux:qux$`),
			},
			expectedOutput: "missing tags: ^qux:qux$",
		},
		{
			name: "Several mismatches",
			actualTags: []string{
				"foo:foo",
				"bar:bar",
				"baz:baz",
				"qux:qux",
				"quux:quux",
			},
			expectedTags: []*regexp.Regexp{
				regexp.MustCompile(`^foo:foo$`),
				regexp.MustCompile(`^bar:`),
				regexp.MustCompile(`:baz$`),
				regexp.MustCompile(`^corge:`),
				regexp.MustCompile(`^grault:`),
			},
			expectedOutput: "unexpected tags: qux:qux, quux:quux\nmissing tags: ^grault:, ^corge:",
		},
		{
			name: "Optional tags",
			actualTags: []string{
				"foo:foo",
				"bar:bar",
				"baz:baz",
				"qux:qux",
			},
			expectedTags: []*regexp.Regexp{
				regexp.MustCompile(`^foo:foo$`),
				regexp.MustCompile(`^bar:`),
				regexp.MustCompile(`:baz$`),
			},
			optionalTags: []*regexp.Regexp{
				regexp.MustCompile(`^qux:qux$`),
				regexp.MustCompile(`^krak:krak$`),
			},
			expectedOutput: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := assertTags(tt.actualTags, tt.expectedTags, tt.optionalTags, tt.acceptUnexpectedTags)
			if output != nil || tt.expectedOutput != "" {
				assert.EqualError(t, output, tt.expectedOutput)
			}
		})
	}
}

func TestParseKindKubernetesVersion(t *testing.T) {
	const defaultVersion = "1.37"

	tests := []struct {
		name     string
		version  string
		expected string
	}{
		{
			name:     "bare version",
			version:  "1.19",
			expected: "1.19",
		},
		{
			name:     "leading v",
			version:  "v1.29",
			expected: "1.29",
		},
		{
			name:     "digest-pinned version",
			version:  "v1.37.1@sha256:cddbfcdc4c30efe7cfb61d5373449ff14d42bf2ddc0c390adf2372c6410fc73f",
			expected: "1.37.1",
		},
		{
			name:     "release candidate",
			version:  "v1.37.0-rc.1@sha256:adc3f1a9ef8b768d311c0592f727c30e274a870c8f0f7713e949558d0275f8f1",
			expected: "1.37.0-rc.1",
		},
		{
			name:     "unset",
			version:  "",
			expected: defaultVersion,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, parseKindKubernetesVersion(tt.version, defaultVersion))
		})
	}
}
