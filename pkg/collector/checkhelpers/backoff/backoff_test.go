// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package backoff

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
)

const (
	operation  = "operation"
	multiplier = 2
	maxBackoff = 10
)

func newStore(op string, s int, m int) *Store {
	// initialize map
	store := New()

	// set status and maximum
	store.backoff[operation] = map[string]int{
		status:  s,
		maximum: m,
	}

	return store
}

func TestRetry(t *testing.T) {
	tests := []struct {
		name           string
		store          *Store
		operationErr   error
		expectedStatus int
		expectedMax    int
	}{
		{
			name:  "No_Error_Thrown_Results_In_No_Backoff_Strategy_Created",
			store: New(),
		},
		{
			name:           "Initial_Error_Thrown_Results_In_Backoff_Strategy_Creation",
			store:          New(),
			operationErr:   fmt.Errorf("error"),
			expectedStatus: 1,
			expectedMax:    1,
		},
		{
			name:           "Mid_Backoff_Error_Results_In_Status_Decrement",
			store:          newStore(operation, 5, 5), // new strategy
			operationErr:   fmt.Errorf("error"),
			expectedStatus: 4, // expected to decrement by 1
			expectedMax:    5,
		},
		{
			name:           "Strategy_Exhaustion_Results_In_New_Strategy",
			store:          newStore(operation, 0, 4), // final attempt
			operationErr:   fmt.Errorf("error"),
			expectedStatus: 8, // expected to double based on maxBackoff
			expectedMax:    8, // expected to double based on maxBackoff
		},
		{
			name:           "Strategy_Exhaustion_Results_In_Exponential_Backoff_Does_Not_Exceed_Maximum",
			store:          newStore(operation, 0, maxBackoff), // final attempt
			operationErr:   fmt.Errorf("error"),
			expectedStatus: maxBackoff, // expected to cap at max
			expectedMax:    maxBackoff, // expected to cap at max
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert := assert.New(t)

			_func := func() (string, error) {
				return "", tt.operationErr
			}

			Retry(tt.store, operation, _func, multiplier, maxBackoff)

			if tt.operationErr == nil {
				// if no error, assert there is no backoff strategy
				assert.NotContains(tt.store.backoff, operation)
			} else {
				// assert underlying map contains operation name
				assert.Contains(tt.store.backoff, operation)

				// assert backoff strategy status is expected
				backoffStatus, _ := tt.store.Get(operation)
				assert.Equal(tt.expectedStatus, backoffStatus)

				// assert backoff strategy maximum is calculated appropriately
				backoffMax, _ := tt.store.backoff[operation][maximum]
				assert.Equal(tt.expectedMax, backoffMax)
			}

		})
	}
}
