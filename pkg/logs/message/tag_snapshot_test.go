// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package message

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

func TestTagSnapshotterReuse(t *testing.T) {
	source := sources.NewLogSource("", &config.LogsConfig{Tags: []string{"env:prod"}, SourceCategory: "cat"})
	static := []string{"filename:a.log"}
	var s TagSnapshotter

	first := s.Get(source, static, []string{"team:a"})
	assert.Same(t, first, s.Get(source, static, []string{"team:a"}))
	assert.Equal(t, []string{"filename:a.log", "team:a", "env:prod", "sourcecategory:cat"}, slices.Collect(first.All()))

	assert.NotSame(t, first, s.Get(source, static, []string{"team:b"}))
	assert.NotSame(t, first, s.Get(source, static, nil))
	assert.NotSame(t, first, s.Get(source, []string{"filename:a.log"}, []string{"team:a"}))

	last := s.Get(source, static, nil)
	source.Config.Tags = []string{"env:dev"}
	assert.NotSame(t, last, s.Get(source, static, nil))

	last = s.Get(source, static, nil)
	source.Config.Service = "web"
	withService := s.Get(source, static, nil)
	assert.NotSame(t, last, withService)
	assert.Contains(t, slices.Collect(withService.All()), "service:web")

	o := NewOrigin(source)
	o.SetService("journal")
	o.SetTagsWithSnapshot(nil, withService, nil)
	_, _, service, ok := o.TagSnapshot()
	assert.True(t, ok)
	assert.Empty(t, service, "the configured service overrides the message's and is already in the snapshot")
}

func TestOriginTagSnapshot(t *testing.T) {
	source := sources.NewLogSource("", &config.LogsConfig{Tags: []string{"env:prod"}})
	var s TagSnapshotter
	o := NewOrigin(source)
	o.SetTagsWithSnapshot([]string{"team:a", "stream:stderr"}, s.Get(source, nil, []string{"team:a"}), []string{"stream:stderr"})

	shared, msgTags, service, ok := o.TagSnapshot()
	assert.True(t, ok)
	assert.NotNil(t, shared)
	assert.Equal(t, []string{"stream:stderr"}, msgTags)
	assert.Empty(t, service)

	source.Config.SourceCategory = "changed"
	_, _, _, ok = o.TagSnapshot()
	assert.False(t, ok)
	source.Config.SourceCategory = ""

	o.SetService("journal")
	_, _, service, ok = o.TagSnapshot()
	assert.True(t, ok)
	assert.Equal(t, "journal", service)
	o.SetService("")

	o.SetTags([]string{"team:a"})
	_, _, _, ok = o.TagSnapshot()
	assert.False(t, ok)

	var nilOrigin *Origin
	_, _, _, ok = nilOrigin.TagSnapshot()
	assert.False(t, ok)
}
