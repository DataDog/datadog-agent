// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package common

import (
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/patchtest"
)

func TestMultiMutator(t *testing.T) {
	tests := []struct {
		name            string
		mutators        []PatchMutator
		expectedMutated bool
		expectedErr     bool
	}{
		{
			name:            "no mutators",
			mutators:        []PatchMutator{},
			expectedMutated: false,
			expectedErr:     false,
		},
		{
			name:            "one mutator, no error, no mutation",
			mutators:        []PatchMutator{&MockMutator{ShoudErr: false, ShouldMutate: false}},
			expectedMutated: false,
			expectedErr:     false,
		},
		{
			name:            "one mutator, no error, mutation",
			mutators:        []PatchMutator{&MockMutator{ShoudErr: false, ShouldMutate: true}},
			expectedMutated: true,
			expectedErr:     false,
		},
		{
			name:            "one mutator, error",
			mutators:        []PatchMutator{&MockMutator{ShoudErr: true, ShouldMutate: false}},
			expectedMutated: false,
			expectedErr:     true,
		},
		{
			name:            "two mutators, no error, no mutation",
			mutators:        []PatchMutator{&MockMutator{ShoudErr: false, ShouldMutate: false}, &MockMutator{ShoudErr: false, ShouldMutate: false}},
			expectedMutated: false,
			expectedErr:     false,
		},
		{
			name:            "two mutators, no error, first mutates",
			mutators:        []PatchMutator{&MockMutator{ShoudErr: false, ShouldMutate: true}, &MockMutator{ShoudErr: false, ShouldMutate: false}},
			expectedMutated: true,
			expectedErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mm := NewPatchMutators(tt.mutators...)
			mutated, err := patchtest.Run(&corev1.Pod{}, "ns", nil, mm.PlanPod)
			if mutated != tt.expectedMutated {
				t.Errorf("MutatePod() = %v, want %v", mutated, tt.expectedMutated)
			}
			if err != nil && !tt.expectedErr {
				t.Errorf("MutatePod() = %v, want %v", err, tt.expectedErr)
			}
		})
	}
}
