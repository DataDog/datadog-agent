// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package diagnose

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const includeTestSuite = "connectivity-datadog-event-platform"

func TestParseIncludeOutput(t *testing.T) {
	for _, tc := range []struct {
		name, format, output string
	}{
		{"json", "json", `{"runs":[{"suite_name":"connectivity-datadog-event-platform","diagnoses":[{"result":1,"rawerror":"context deadline exceeded"}]}],"summary":{"total":1,"fail":1}}`},
		{"text", "text", "=== Starting diagnose ===\n==============\nSuite: connectivity-datadog-event-platform\n\n1. --------------\n  FAIL Connectivity\n  Diagnosis: Connection failed\n  Error: context deadline exceeded\n\n-------------------------\n  Total:1, Fail:1\n"},
		{"windows text", "text", "Suite: connectivity-datadog-event-platform\r\n  FAIL Connectivity\r\n  Error: context deadline exceeded\r\n  Total:1, Fail:1\r\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := parseIncludeOutput(tc.output, tc.format)
			require.NoError(t, err)
			require.NoError(t, result.validate([]string{includeTestSuite}))
			assert.True(t, result.hasOnlyTransientFailures())
		})
	}
	for _, tc := range []struct{ format, output string }{
		{"json", `{}`}, {"json", `{`}, {"text", "not diagnose output"},
	} {
		_, err := parseIncludeOutput(tc.output, tc.format)
		assert.Error(t, err)
	}
}

func TestIncludeOutputValidation(t *testing.T) {
	healthy := includeFixture(0, "")
	assert.NoError(t, healthy.validate([]string{includeTestSuite}))
	assert.Error(t, healthy.validate([]string{"port-conflict"}))
	assert.Error(t, healthy.validate([]string{includeTestSuite, "port-conflict"}))
	assert.NoError(t, (includeOutput{}).validate([]string{"check-datadog"}))
	assert.Error(t, (includeOutput{}).validate([]string{includeTestSuite}))
	assert.Error(t, (includeOutput{Runs: []includeRun{{Name: includeTestSuite}}}).validate([]string{includeTestSuite}))
	healthy.Summary.Success = 0
	assert.Error(t, healthy.validate([]string{includeTestSuite}))
}

func TestIncludeIdentities(t *testing.T) {
	healthy := includeFixture(0, "")
	timeout := includeFixture(1, "context deadline exceeded")
	assert.Equal(t, healthy.identities(), timeout.identities(), "network outcomes must not affect selection equivalence")
	timeout.Runs[0].Diagnoses = append(timeout.Runs[0].Diagnoses, timeout.Runs[0].Diagnoses[0])
	assert.NotEqual(t, healthy.identities(), timeout.identities(), "a missing or duplicate diagnosis must fail equivalence")
	changed := includeFixture(0, "")
	changed.Runs[0].Diagnoses[0].Name = "different endpoint"
	assert.NotEqual(t, healthy.identities(), changed.identities())
}

func TestRunIncludeSelectionRetry(t *testing.T) {
	timeout := includeFixture(1, "context deadline exceeded (Client.Timeout exceeded while awaiting headers)")
	healthy := includeFixture(0, "")
	for _, tc := range []struct {
		name      string
		first     includeOutput
		firstErr  error
		wantCalls int
		wantError bool
	}{
		{"healthy", healthy, nil, 1, false},
		{"timeout", timeout, nil, 2, false},
		{"non-transient failure", includeFixture(1, "HTTP 403"), nil, 1, false},
		{"unexpected error", includeFixture(3, "context deadline exceeded"), nil, 1, false},
		{"parse error", includeOutput{}, errors.New("invalid JSON"), 1, true},
		{"selection error", includeOutput{}, nil, 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			retryAvailable, calls := true, 0
			result, err := runIncludeSelection(func() (includeOutput, error) {
				calls++
				if calls == 1 {
					return tc.first, tc.firstErr
				}
				return healthy, nil
			}, []string{includeTestSuite}, &retryAvailable)
			assert.Equal(t, tc.wantCalls, calls)
			assert.Equal(t, tc.wantError, err != nil)
			assert.Equal(t, tc.wantCalls == 1, retryAvailable)
			if tc.wantCalls == 2 {
				assert.Zero(t, result.Summary.Fail)
			}
		})
	}

	// The shared token permits only one extra command across different cases.
	retryAvailable, calls := true, 0
	for range 2 {
		result, err := runIncludeSelection(func() (includeOutput, error) {
			calls++
			return timeout, nil
		}, []string{includeTestSuite}, &retryAvailable)
		require.NoError(t, err)
		assert.Equal(t, 1, result.Summary.Fail, "persistent failures must remain visible")
	}
	assert.Equal(t, 3, calls)

	// A timeout on a successful diagnosis must not excuse another failure.
	mixed := includeFixture(1, "HTTP 403")
	mixed.Runs[0].Diagnoses = append(mixed.Runs[0].Diagnoses, includeDiagnosis{Result: 0, RawError: "context deadline exceeded"})
	mixed.Summary.Total++
	mixed.Summary.Success++
	assert.False(t, mixed.hasOnlyTransientFailures())
}

func includeFixture(status int, rawError string) includeOutput {
	summary := counters{Total: 1}
	switch status {
	case 0:
		summary.Success = 1
	case 1:
		summary.Fail = 1
	case 3:
		summary.UnexpectedErr = 1
	}
	return includeOutput{
		Runs:    []includeRun{{Name: includeTestSuite, Diagnoses: []includeDiagnosis{{Result: status, RawError: rawError}}}},
		Summary: summary,
	}
}
