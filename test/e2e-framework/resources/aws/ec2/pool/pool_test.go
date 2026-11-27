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
)

type fakeAPIError struct{ code string }

func (e fakeAPIError) Error() string     { return e.code }
func (e fakeAPIError) ErrorCode() string { return e.code }

func TestIsImageNotFound(t *testing.T) {
	cases := map[string]struct {
		err  error
		want bool
	}{
		"no root snapshot": {fmt.Errorf("%w: image ami-1 has no root snapshot", errImageNotFound), true},
		"ami not found":    {fmt.Errorf("describe: %w", fakeAPIError{"InvalidAMIID.NotFound"}), true},
		"ami unavailable":  {fakeAPIError{"InvalidAMIID.Unavailable"}, true},
		"throttled":        {fakeAPIError{"RequestLimitExceeded"}, false},
		"unauthorized":     {fakeAPIError{"UnauthorizedOperation"}, false},
		"network error":    {errors.New("dial tcp: connection refused"), false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, isImageNotFound(tc.err))
		})
	}
}
