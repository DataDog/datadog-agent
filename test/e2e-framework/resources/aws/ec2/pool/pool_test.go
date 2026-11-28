// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package pool

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRevertSkipReason(t *testing.T) {
	tests := []struct {
		name       string
		lease      leaseRecord
		devMode    bool
		wantReason string
		wantErr    error
	}{
		{
			name:  "baseline image requires a revert",
			lease: leaseRecord{Status: statusInUse, ImageID: "ami-0123456789abcdef0"},
		},
		{
			name:       "persistent lease skips the revert",
			lease:      leaseRecord{Status: statusInUse, ImageID: "ami-0123456789abcdef0", Persistent: true},
			wantReason: "persistent lease",
		},
		{
			name:       "persistent lease without image skips the revert",
			lease:      leaseRecord{Status: statusInUse, Persistent: true},
			wantReason: "persistent lease",
		},
		{
			name:       "dev mode skips the revert",
			lease:      leaseRecord{Status: statusInUse, ImageID: "ami-0123456789abcdef0"},
			devMode:    true,
			wantReason: "dev mode",
		},
		{
			name:       "dev mode without image skips the revert",
			lease:      leaseRecord{Status: statusInUse},
			devMode:    true,
			wantReason: "dev mode",
		},
		{
			name:    "missing image fails instead of silently skipping the revert",
			lease:   leaseRecord{Status: statusInUse},
			wantErr: ErrNoBaselineImage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason, err := revertSkipReason(tt.lease, tt.devMode)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantReason, reason)
		})
	}
}

type fakeAPIError struct{ code string }

func (e fakeAPIError) Error() string     { return e.code }
func (e fakeAPIError) ErrorCode() string { return e.code }

func TestIsImageNotFound(t *testing.T) {
	assert.True(t, isImageNotFound(fakeAPIError{code: "InvalidAMIID.NotFound"}))
	assert.True(t, isImageNotFound(fmt.Errorf("describe: %w", fakeAPIError{code: "InvalidAMIID.Unavailable"})))
	assert.False(t, isImageNotFound(fakeAPIError{code: "RequestLimitExceeded"}))
	assert.False(t, isImageNotFound(fakeAPIError{code: "UnauthorizedOperation"}))
	assert.False(t, isImageNotFound(errors.New("connection reset")))
}
