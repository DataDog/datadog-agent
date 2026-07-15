// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func TestNormalizeVolumes(t *testing.T) {
	volA := corev1.Volume{
		Name: "vol-a",
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
	volB := corev1.Volume{
		Name: "vol-b",
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
	volADifferentSource := corev1.Volume{
		Name: "vol-a",
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{Path: "/tmp"},
		},
	}

	tests := []struct {
		name     string
		input    []corev1.Volume
		expected []corev1.Volume
		wantErr  bool
	}{
		{
			name:     "no volumes",
			input:    nil,
			expected: nil,
			wantErr:  false,
		},
		{
			name:     "single volume, no duplicates",
			input:    []corev1.Volume{volA},
			expected: []corev1.Volume{volA},
			wantErr:  false,
		},
		{
			name:     "multiple unique volumes",
			input:    []corev1.Volume{volA, volB},
			expected: []corev1.Volume{volA, volB},
			wantErr:  false,
		},
		{
			name:     "exact duplicate is removed",
			input:    []corev1.Volume{volA, volA},
			expected: []corev1.Volume{volA},
			wantErr:  false,
		},
		{
			name:     "multiple exact duplicates are removed",
			input:    []corev1.Volume{volA, volB, volA, volB},
			expected: []corev1.Volume{volA, volB},
			wantErr:  false,
		},
		{
			name:     "same name but different fields returns error",
			input:    []corev1.Volume{volA, volADifferentSource},
			expected: []corev1.Volume{volA, volADifferentSource},
			wantErr:  true,
		},
		{
			name:     "duplicate with unique volume preserved",
			input:    []corev1.Volume{volA, volB, volA},
			expected: []corev1.Volume{volA, volB},
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			volumes, err := normalizeVolumes(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("unexpected err: %q", err)
			}

			// assert elements match as if there is an error normalizing the volumes, no changes should occur
			assert.ElementsMatch(t, volumes, tt.expected, "normalization failed, volumes do not match")
		})
	}
}
