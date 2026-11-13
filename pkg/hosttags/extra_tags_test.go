// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package hosttags

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type staticGetter []string

func (s staticGetter) GetHostTags() []string { return s }

func TestWithExtraTags(t *testing.T) {
	tests := []struct {
		name  string
		base  []string
		extra []string
		want  []string
	}{
		{name: "no tags", want: nil},
		{name: "base only", base: []string{"env:prod"}, want: []string{"env:prod"}},
		{name: "extra only", extra: []string{"task_arn:arn"}, want: []string{"task_arn:arn"}},
		{name: "base and extra", base: []string{"env:prod"}, extra: []string{"task_arn:arn"}, want: []string{"env:prod", "task_arn:arn"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := WithExtraTags(staticGetter(tt.base), func() []string { return tt.extra })
			assert.Equal(t, tt.want, g.GetHostTags())
		})
	}
}

func TestWithExtraTagsDoesNotModifyBase(t *testing.T) {
	base := make([]string, 1, 4)
	base[0] = "env:prod"
	g := WithExtraTags(staticGetter(base), func() []string { return []string{"task_arn:arn"} })

	_ = g.GetHostTags()
	assert.Equal(t, []string{"env:prod"}, base)
	assert.Equal(t, []string{"env:prod", ""}, base[:2], "the base slice's spare capacity must not be written")
}
