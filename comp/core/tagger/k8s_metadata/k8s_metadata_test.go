// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package k8smetadata

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/tagger/taglist"
)

func TestMetadataAsTags(t *testing.T) {
	tests := []struct {
		name           string
		k              string
		v              string
		metadataAsTags map[string]string
		want           []string
	}{
		{
			name:           "nominal case",
			k:              "foo",
			v:              "bar",
			metadataAsTags: map[string]string{"foo": "foo"},
			want:           []string{"foo:bar"},
		},
		{
			name:           "label tpl var",
			k:              "foo",
			v:              "bar",
			metadataAsTags: map[string]string{"foo": "%%label%%"},
			want:           []string{"foo:bar"},
		},
		{
			name:           "annotation tpl var",
			k:              "foo",
			v:              "bar",
			metadataAsTags: map[string]string{"foo": "%%annotation%%"},
			want:           []string{"foo:bar"},
		},
		{
			name:           "env tpl var",
			k:              "foo",
			v:              "bar",
			metadataAsTags: map[string]string{"foo": "%%env%%"},
			want:           []string{"foo:bar"},
		},
		{
			name:           "label tpl var with prefix",
			k:              "foo",
			v:              "bar",
			metadataAsTags: map[string]string{"foo": "prefix_%%label%%"},
			want:           []string{"prefix_foo:bar"},
		},
		{
			name:           "annotation tpl var with suffix",
			k:              "foo",
			v:              "bar",
			metadataAsTags: map[string]string{"foo": "%%annotation%%_suffix"},
			want:           []string{"foo_suffix:bar"},
		},
		{
			name:           "env tpl var with suffix",
			k:              "foo",
			v:              "bar",
			metadataAsTags: map[string]string{"foo": "%%env%%_suffix"},
			want:           []string{"foo_suffix:bar"},
		},
		{
			name:           "with wildcard",
			k:              "foo",
			v:              "bar",
			metadataAsTags: map[string]string{"fo*": "baz"},
			want:           []string{"baz:bar"},
		},
		{
			name:           "match all labels",
			k:              "foo",
			v:              "bar",
			metadataAsTags: map[string]string{"*": "%%label%%"},
			want:           []string{"foo:bar"},
		},
		{
			name:           "match all annotations",
			k:              "foo",
			v:              "bar",
			metadataAsTags: map[string]string{"*": "%%annotation%%"},
			want:           []string{"foo:bar"},
		},
		{
			name:           "match all envs",
			k:              "foo",
			v:              "bar",
			metadataAsTags: map[string]string{"*": "%%env%%"},
			want:           []string{"foo:bar"},
		},
		{
			name:           "nominal split case",
			k:              "foo",
			v:              "bar",
			metadataAsTags: map[string]string{"foo": "foo, app_foo"},
			want:           []string{"foo:bar", "app_foo:bar"},
		},
		{
			name:           "label tpl var with normal label",
			k:              "foo",
			v:              "bar",
			metadataAsTags: map[string]string{"foo": "%%label%% , app_foo"},
			want:           []string{"foo:bar", "app_foo:bar"},
		},
		{
			name:           "normal label and annotation tpl var with suffix",
			k:              "foo",
			v:              "bar",
			metadataAsTags: map[string]string{"foo": "app_foo,%%annotation%%_suffix"},
			want:           []string{"app_foo:bar", "foo_suffix:bar"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tagList := taglist.NewTagList()
			m, g := InitMetadataAsTags(tt.metadataAsTags)
			AddMetadataAsTags(tt.k, tt.v, m, g, tagList)
			tags, _, _, _ := tagList.Compute()
			assert.ElementsMatch(t, tt.want, tags)
		})
	}
}

func TestResolveTag(t *testing.T) {
	testCases := []struct {
		tmpl, label, expected string
	}{
		{
			"kube_%%label%%", "app", "kube_app",
		},
		{
			"foo_%%label%%_bar", "app", "foo_app_bar",
		},
		{
			"%%label%%%%label%%", "app", "appapp",
		},
		{
			"kube_%%annotation%%", "app", "kube_app",
		},
		{
			"foo_%%annotation%%_bar", "app", "foo_app_bar",
		},
		{
			"%%annotation%%%%annotation%%", "app", "appapp",
		},
		{
			"kube_", "app", "kube_", // no template variable
		},
		{
			"kube_%%foo%%", "app", "kube_", // unsupported template variable
		},
	}

	for i, testCase := range testCases {
		t.Run(fmt.Sprintf("#%d", i), func(t *testing.T) {
			tagName := resolveTag(testCase.tmpl, testCase.label)
			assert.Equal(t, testCase.expected, tagName)
		})
	}
}
