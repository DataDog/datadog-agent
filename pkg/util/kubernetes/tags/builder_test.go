// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package tags

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_tagListBuilder_buildTag(t *testing.T) {
	tests := []struct {
		name string
		k    string
		v    string
		want string
	}{
		{
			name: "nominal case",
			k:    "foo",
			v:    "bar",
			want: "foo:bar",
		},
		{
			name: "empty key",
			k:    "",
			v:    "bar",
			want: "",
		},
		{
			name: "empty value",
			k:    "foo",
			v:    "",
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tlb := newTagListBuilder()
			assert.Equal(t, tt.want, tlb.buildTag(tt.k, tt.v))
			assert.Equal(t, 0, tlb.sb.Len())
		})
	}
}

func Test_tagListBuilder_addNotEmpty(t *testing.T) {
	tests := []struct {
		name string
		k    string
		v    string
		want []string
	}{
		{
			name: "nominal case",
			k:    "foo",
			v:    "bar",
			want: []string{"foo:bar"},
		},
		{
			name: "empty key",
			k:    "",
			v:    "bar",
			want: []string{},
		},
		{
			name: "empty value",
			k:    "foo",
			v:    "",
			want: []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tlb := newTagListBuilder()
			tlb.addNotEmpty(tt.k, tt.v)
			assert.ElementsMatch(t, tt.want, tlb.tags())
		})
	}
}
