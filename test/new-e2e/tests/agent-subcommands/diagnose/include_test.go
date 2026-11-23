// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package diagnose

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const textFailure = "Suite: connectivity-datadog-event-platform\n  FAIL Connectivity\n  Diagnosis: remote response\n  Error: context deadline exceeded\n"
const textFailureSummary = "-------------------------\n  Total:1, Fail:1\n"

func TestIncludeIdentities(t *testing.T) {
	for _, tc := range []struct{ name, format, output string }{
		{"json", "json", `{"runs":[{"suite_name":"connectivity-datadog-event-platform","diagnoses":[{"name":"Connectivity","result":1,"rawerror":"timeout"}]}],"summary":{"total":1,"fail":1}}`},
		{"text", "text", textFailure + textFailureSummary},
		{"windows text", "text", strings.ReplaceAll(textFailure+textFailureSummary, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reference, err := parseIncludeOutput(tc.output, tc.format)
			require.NoError(t, err)
			// Reproduce the original flake: the default request timed out, but
			// including all suites selected the same checks and succeeded.
			healthy := strings.NewReplacer(`"result":1`, `"result":0`, `"fail":1`, `"success":1`, "FAIL", "PASS", "Fail:1", "Success:1").Replace(tc.output)
			selected, err := parseIncludeOutput(healthy, tc.format)
			require.NoError(t, err)
			assert.Equal(t, 1, reference.Summary.Fail)
			assert.Zero(t, selected.Summary.Fail)
			require.Len(t, reference.identities(), 1)
			assert.Equal(t, reference.identities(), selected.identities())
			selected.Runs[0].Diagnoses = append(selected.Runs[0].Diagnoses, selected.Runs[0].Diagnoses[0])
			assert.NotEqual(t, reference.identities(), selected.identities(), "duplicate diagnoses must fail equivalence")
			selected.Runs[0].Diagnoses = nil
			assert.NotEqual(t, reference.identities(), selected.identities(), "missing diagnoses must fail equivalence")
		})
	}
}

func TestParseIncludeTextSummary(t *testing.T) {
	const embedded = "Total:999\n-------------------------\n  Total:42, Success:42\n"
	for _, tc := range []struct {
		name, output string
		wantError    bool
	}{
		{"embedded counters", textFailure + embedded + textFailureSummary, false},
		{"windows embedded counters", strings.ReplaceAll(textFailure+embedded+textFailureSummary, "\n", "\r\n"), false},
		{"no final newline", textFailure + strings.TrimSuffix(textFailureSummary, "\n"), false},
		{"missing final summary", textFailure + "Total:1, Fail:1\n", true},
		{"malformed final summary", textFailure + embedded + "-------------------------\n  Total:invalid\n", true},
		{"trailing text", textFailure + embedded + textFailureSummary + "unexpected trailer\n", true},
		{"trailing summary fields", textFailure + "-------------------------\n  Total:1, Fail:1, ignored:9\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := parseIncludeOutput(tc.output, "text")
			if tc.wantError {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, counters{Total: 1, Fail: 1}, *result.Summary)
		})
	}
	for _, output := range []string{`{}`, `{`} {
		_, err := parseIncludeOutput(output, "json")
		assert.Error(t, err)
	}
}
