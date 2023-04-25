// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package activity_tree

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_ExtractFirstParent(t *testing.T) {
	type args struct {
		path string
	}
	tests := []struct {
		name  string
		args  args
		want  string
		want1 int
	}{
		{
			name:  "empty path",
			args:  args{},
			want:  "",
			want1: 0,
		},
		{
			name:  "root path",
			args:  args{"/"},
			want:  "",
			want1: 0,
		},
		{
			name:  "one char with slash",
			args:  args{"/h"},
			want:  "h",
			want1: 2,
		},
		{
			name:  "one char no slash",
			args:  args{"h"},
			want:  "h",
			want1: 1,
		},
		{
			name:  "single node with slash",
			args:  args{"/hello"},
			want:  "hello",
			want1: 6,
		},
		{
			name:  "single node no slash",
			args:  args{"hello"},
			want:  "hello",
			want1: 5,
		},
		{
			name:  "parent with slash",
			args:  args{"/hello/there/"},
			want:  "hello",
			want1: 6,
		},
		{
			name:  "parent no slash",
			args:  args{"hello/there/"},
			want:  "hello",
			want1: 5,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := ExtractFirstParent(tt.args.path)
			assert.Equalf(t, tt.want, got, "ExtractFirstParent(%v)", tt.args.path)
			assert.Equalf(t, tt.want1, got1, "ExtractFirstParent(%v)", tt.args.path)
		})
	}
}
