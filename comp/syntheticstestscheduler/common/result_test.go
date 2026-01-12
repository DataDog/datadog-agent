// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package common

import (
	"testing"
)

func TestAssertionResult_Compare(t *testing.T) {
	tests := map[Operator][]struct {
		name     string
		expected interface{}
		actual   interface{}
		want     bool
		wantErr  bool
	}{
		OperatorLessThan: {
			{"actual less", 10, 5, true, false},
			{"actual equal", 10, 10, false, false},
			{"actual more", 10, 20, false, false},
		},
		OperatorLessThanOrEqual: {
			{"less is true", 10, 5, true, false},
			{"equal is true", 10, 10, true, false},
			{"more is false", 10, 20, false, false},
		},
		OperatorMoreThan: {
			{"more is true", 10, 20, true, false},
			{"equal is false", 10, 10, false, false},
			{"less is false", 10, 5, false, false},
		},
		OperatorMoreThanOrEqual: {
			{"more is true", 10, 20, true, false},
			{"equal is true", 10, 10, true, false},
			{"less is false", 10, 5, false, false},
			{"float equal", 5.5, 5.5, true, false},
		},
		OperatorIs: {
			{"equal ints", "10", "10", true, false},
			{"not equal", "10", "20", false, false},
			{"equal numeric different format", "100.0", "100", true, false},
			{"string equal", "foo", "foo", true, false},
			{"string not equal", "foo", "bar", false, false},
		},
		OperatorIsNot: {
			{"not equal", 10, 20, true, false},
			{"equal", 10, 10, false, false},
		},
	}

	for op, cases := range tests {
		t.Run(string(op), func(t *testing.T) {
			for _, tt := range cases {
				t.Run(tt.name, func(t *testing.T) {
					ar := &AssertionResult{
						Operator: op,
						Expected: tt.expected,
						Actual:   tt.actual,
					}
					err := ar.Compare()
					if (err != nil) != tt.wantErr {
						t.Fatalf("Compare() error = %v, wantErr %v", err, tt.wantErr)
					}
					if !tt.wantErr && ar.Valid != tt.want {
						t.Errorf("Compare() got Valid = %v, want %v", ar.Valid, tt.want)
					}
				})
			}
		})
	}

	// Explicit test for unsupported operator
	t.Run("unsupported operator", func(t *testing.T) {
		ar := &AssertionResult{
			Operator: "invalidOperator",
			Expected: 10,
			Actual:   10,
		}
		err := ar.Compare()
		if err == nil {
			t.Fatalf("expected error for unsupported operator")
		}
	})
}
