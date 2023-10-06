// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_merge(t *testing.T) {
	tests := []struct {
		name string
		s1   []string
		s2   []string
		want []string
	}{
		{
			name: "nominal case",
			s1:   []string{"foo", "bar"},
			s2:   []string{"baz", "tar"},
			want: []string{"foo", "bar", "baz", "tar"},
		},
		{
			name: "empty s1",
			s1:   []string{},
			s2:   []string{"baz", "tar"},
			want: []string{"baz", "tar"},
		},
		{
			name: "empty s2",
			s1:   []string{"foo", "bar"},
			s2:   []string{},
			want: []string{"foo", "bar"},
		},
		{
			name: "dedupe 1",
			s1:   []string{"foo", "bar"},
			s2:   []string{"baz", "foo"},
			want: []string{"foo", "bar", "baz"},
		},
		{
			name: "dedupe 2",
			s1:   []string{"foo", "foo"},
			s2:   []string{"foo", "foo"},
			want: []string{"foo"},
		},
		{
			name: "dedupe 3",
			s1:   []string{"foo", "foo"},
			s2:   []string{"baz", "tar"},
			want: []string{"foo", "baz", "tar"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.EqualValues(t, tt.want, merge(tt.s1, tt.s2))
		})
	}
}
