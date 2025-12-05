// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package namer

import (
	"math"
	"strconv"
	"strings"
	"testing"

	fuzz "github.com/google/gofuzz"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJoinWithMaxLength(t *testing.T) {
	for i, tt := range []struct {
		maxLength int
		tokens    []string
		noHash    bool
		expected  string
	}{
		// No truncation needed
		{
			maxLength: math.MaxInt,
			tokens:    []string{"foo", "bar", "baz"},
			expected:  "foo-bar-baz",
		},
		// Transition from full format to truncated format
		{
			maxLength: 20,
			tokens:    []string{"foo", "bar", "baz", "qux", "quux"},
			expected:  "foo-bar-baz-qux-quux",
		},
		{
			maxLength: 19,
			tokens:    []string{"foo", "bar", "baz", "qux", "quux"},
			expected:  "fo-ba-ba-qu-quu-100",
		},
		// Transition from truncated format to hash only
		{
			maxLength: 11,
			tokens:    []string{"foo", "bar", "baz", "qux", "quux"},
			expected:  "10087cd4460",
		},
		{
			maxLength: 10,
			tokens:    []string{"foo", "bar", "baz", "qux", "quux"},
			expected:  "10087cd446",
		},
		// Max length too small
		// Defaults to hash only
		{
			maxLength: 4,
			tokens:    []string{"foo", "bar", "baz", "qux", "quux"},
			expected:  "1008",
		},
		// Truncation is applied to tokens proportionally to their size
		{
			maxLength: 23,
			tokens:    []string{"FfffOoooOooo", "BbbAaaRrr", "BbAaZz", "QUX"},
			noHash:    true,
			expected:  "FfffOooo-BbbAaa-BbAa-QU",
		},
		{
			maxLength: 13,
			tokens:    []string{"FfffOoooOooo", "BbbAaaRrr", "BbAaZz", "QUX"},
			noHash:    true,
			expected:  "Ffff-Bbb-Bb-Q",
		},
		// Truncation are spread best effort evenly on all tokens
		{
			maxLength: 18,
			tokens:    []string{"foo", "bar", "baz", "qux", "qux"},
			noHash:    true,
			expected:  "foo-bar-ba-qux-qux",
		},
		{
			maxLength: 17,
			tokens:    []string{"foo", "bar", "baz", "qux", "qux"},
			noHash:    true,
			expected:  "foo-ba-baz-qu-qux",
		},
		{
			maxLength: 16,
			tokens:    []string{"foo", "bar", "baz", "qux", "qux"},
			noHash:    true,
			expected:  "fo-bar-ba-qux-qu",
		},
		{
			maxLength: 15,
			tokens:    []string{"foo", "bar", "baz", "qux", "qux"},
			noHash:    true,
			expected:  "fo-ba-baz-qu-qu",
		},
		{
			maxLength: 14,
			tokens:    []string{"foo", "bar", "baz", "qux", "qux"},
			noHash:    true,
			expected:  "fo-ba-ba-qu-qu",
		},
		{
			maxLength: 13,
			tokens:    []string{"foo", "bar", "baz", "qux", "qux"},
			noHash:    true,
			expected:  "fo-ba-b-qu-qu",
		},
		{
			maxLength: 12,
			tokens:    []string{"foo", "bar", "baz", "qux", "qux"},
			noHash:    true,
			expected:  "fo-b-ba-q-qu",
		},
		{
			maxLength: 11,
			tokens:    []string{"foo", "bar", "baz", "qux", "qux"},
			noHash:    true,
			expected:  "f-ba-b-qu-q",
		},
		{
			maxLength: 10,
			tokens:    []string{"foo", "bar", "baz", "qux", "qux"},
			noHash:    true,
			expected:  "f-b-ba-q-q",
		},
		{
			maxLength: 9,
			tokens:    []string{"foo", "bar", "baz", "qux", "qux"},
			noHash:    true,
			expected:  "f-b-b-q-q",
		},
		// Some real cases with a stack name from the CI
		// 37 is the maximum size of EKS node group names
		{
			maxLength: 37,
			tokens:    []string{"ci-17317712-4670-eks-cluster", "linux", "ng"},
			expected:  "ci-17317712-4670-eks-cluster-linux-ng", // No truncation needed
		},
		{
			maxLength: 37,
			tokens:    []string{"ci-17317712-4670-eks-cluster", "linux-arm", "ng"},
			expected:  "ci-17317712-4670-ek-linux-a-n-45816b0",
		},
		{
			maxLength: 37,
			tokens:    []string{"ci-17317712-4670-eks-cluster", "bottlerocket", "ng"},
			expected:  "ci-17317712-4670-e-bottlero-n-a2e7bad",
		},
		// 32 is the maximum size of load-balancer names
		{
			maxLength: 32,
			tokens:    []string{"ci-17317712-4670-eks-cluster", "fakeintake"},
			expected:  "ci-17317712-4670-e-fakein-5f12e1",
		},
		{
			maxLength: 32,
			tokens:    []string{"ci-17317712-4670-eks-cluster", "nginx"},
			expected:  "ci-17317712-4670-eks-ngin-db3fe1",
		},
		{
			maxLength: 32,
			tokens:    []string{"ci-796640089-4670-e2e-otlpingestopnamev2remappingtestsuite-1607f1ae0274c934", "fakeintake"},
			expected:  "ci-796640089-4670-e2e-fak-bbb43c",
		},
		{
			maxLength: 32,
			tokens:    []string{"ci-796640089-4670-e2e-otelagentspanreceiverv2testsuite-68e83930ca520340", "fakeintake"},
			expected:  "ci-796640089-4670-e2e-fak-bbb969",
		},
	} {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			require.Conditionf(t, func() bool { return outputOK(tt.maxLength, tt.tokens, tt.expected) }, "Expected output string %q doesn’t match expected properties", tt.expected)
			noHash = tt.noHash
			output := joinWithMaxLength(tt.maxLength, tt.tokens)
			assert.Equal(t, tt.expected, output)
		})
	}
}

func outputOK(maxLength int, tokens []string, output string) bool {
	full := strings.Join(tokens, "-")
	if len(full) <= maxLength {
		return output == full
	}

	return len(output) == maxLength
}

func FuzzJoinWithMaxLength(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		f := fuzz.NewFromGoFuzz(data)

		var tt struct {
			maxLength int
			tokens    []string
		}
		f.Fuzz(&tt)

		output := joinWithMaxLength(tt.maxLength, tt.tokens)
		assert.Conditionf(t, func() bool { return outputOK(tt.maxLength, tt.tokens, output) }, "joinWithMaxLength(%d, %v) => %q", tt.maxLength, tt.tokens, output)
	})
}
