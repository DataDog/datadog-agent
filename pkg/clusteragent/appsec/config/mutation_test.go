// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package config

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeOutcome(t *testing.T) {
	mutatedErr := errors.New("mutate failed")
	skippedErr := errors.New("boom")
	errorErr := errors.New("boom")

	tests := []struct {
		name                  string
		outcome               MutationOutcome
		err                   error
		expectedOutcome       MutationOutcome
		expectedReason        string
		expectedRetainedError error
		expectedRetainedText  string
		expectedAdmission     bool
		expectedAdmissionErr  error
		wantAdmissionErr      bool
	}{
		{
			name:              "mutated with nil error admits",
			outcome:           MutationMutated,
			expectedOutcome:   MutationMutated,
			expectedReason:    "none",
			expectedAdmission: true,
		},
		{
			name:                  "mutated with error is promoted to error",
			outcome:               MutationMutated,
			err:                   mutatedErr,
			expectedOutcome:       MutationError,
			expectedReason:        "error",
			expectedRetainedError: mutatedErr,
			expectedAdmissionErr:  mutatedErr,
			wantAdmissionErr:      true,
		},
		{
			name:            "skipped with typed reason swallows error",
			outcome:         MutationSkipped,
			err:             &MutationSkippedReason{Reason: SkipReasonAlreadySidecar},
			expectedOutcome: MutationSkipped,
			expectedReason:  "already_sidecar",
		},
		{
			name:            "skipped with wrapped typed reason unwraps and swallows error",
			outcome:         MutationSkipped,
			err:             fmt.Errorf("wrap: %w", &MutationSkippedReason{Reason: SkipReasonMissingUDSPath}),
			expectedOutcome: MutationSkipped,
			expectedReason:  "missing_uds_path",
		},
		{
			name:                  "skipped with real error is promoted to error",
			outcome:               MutationSkipped,
			err:                   skippedErr,
			expectedOutcome:       MutationError,
			expectedReason:        "error",
			expectedRetainedError: skippedErr,
			expectedRetainedText:  "boom",
			expectedAdmissionErr:  skippedErr,
			wantAdmissionErr:      true,
		},
		{
			name:            "skipped with nil error uses unknown reason",
			outcome:         MutationSkipped,
			expectedOutcome: MutationSkipped,
			expectedReason:  "unknown",
		},
		{
			name:                  "error with error retains error",
			outcome:               MutationError,
			err:                   errorErr,
			expectedOutcome:       MutationError,
			expectedReason:        "error",
			expectedRetainedError: errorErr,
			expectedRetainedText:  "boom",
			expectedAdmissionErr:  errorErr,
			wantAdmissionErr:      true,
		},
		{
			name:                 "error with nil error synthesizes error",
			outcome:              MutationError,
			expectedOutcome:      MutationError,
			expectedReason:       "error",
			expectedRetainedText: "mutation error with nil error",
			wantAdmissionErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			canonical, reason, retained := NormalizeOutcome(tt.outcome, tt.err)

			assert.Equal(t, tt.expectedOutcome, canonical)
			assert.Equal(t, tt.expectedReason, reason)
			if tt.expectedRetainedError != nil {
				require.ErrorIs(t, retained, tt.expectedRetainedError)
			} else if tt.expectedRetainedText != "" {
				require.Error(t, retained)
				assert.Equal(t, tt.expectedRetainedText, retained.Error())
			} else {
				assert.NoError(t, retained)
			}

			admitted, admissionErr := NormalizeOutcomeForAdmission(tt.outcome, tt.err)

			assert.Equal(t, tt.expectedAdmission, admitted)
			if tt.expectedAdmissionErr != nil {
				require.ErrorIs(t, admissionErr, tt.expectedAdmissionErr)
			} else if tt.wantAdmissionErr {
				require.Error(t, admissionErr)
				if tt.expectedRetainedText != "" {
					assert.Equal(t, tt.expectedRetainedText, admissionErr.Error())
				}
			} else {
				assert.NoError(t, admissionErr)
			}
		})
	}
}
