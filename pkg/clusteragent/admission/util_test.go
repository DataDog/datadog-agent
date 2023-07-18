// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package admission

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/version"
)

func Test_shouldFallback(t *testing.T) {
	tests := []struct {
		name    string
		v       *version.Info
		want    bool
		wantErr bool
	}{
		{
			name:    "v1.10 => fallback",
			v:       &version.Info{Major: "1", Minor: "10"},
			want:    true,
			wantErr: false,
		},
		{
			name:    "v1.11 => fallback",
			v:       &version.Info{Major: "1", Minor: "11"},
			want:    true,
			wantErr: false,
		},
		{
			name:    "v1.12 => fallback",
			v:       &version.Info{Major: "1", Minor: "12"},
			want:    true,
			wantErr: false,
		},
		{
			name:    "v1.13 => fallback",
			v:       &version.Info{Major: "1", Minor: "13"},
			want:    true,
			wantErr: false,
		},
		{
			name:    "v1.14 => fallback",
			v:       &version.Info{Major: "1", Minor: "14"},
			want:    true,
			wantErr: false,
		},
		{
			name:    "v1.15 => no fallback",
			v:       &version.Info{Major: "1", Minor: "15+"},
			want:    false,
			wantErr: false,
		},
		{
			name:    "v1.9 => no fallback",
			v:       &version.Info{Major: "1", Minor: "9"},
			want:    false,
			wantErr: false,
		},
		{
			name:    "unsupported major #1",
			v:       &version.Info{Major: "0", Minor: "14"},
			want:    false,
			wantErr: false,
		},
		{
			name:    "unsupported major #2",
			v:       &version.Info{Major: "2", Minor: "14"},
			want:    false,
			wantErr: false,
		},
		{
			name:    "invalid minor",
			v:       &version.Info{Major: "1", Minor: "foo"},
			want:    false,
			wantErr: true,
		},
		{
			name:    "custom minor",
			v:       &version.Info{Major: "1", Minor: "10+"},
			want:    true,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := shouldFallback(tt.v)
			assert.Equal(t, tt.wantErr, err != nil)
			assert.Equal(t, tt.want, got)
		})
	}
}
