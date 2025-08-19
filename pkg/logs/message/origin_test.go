// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package message

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

func TestSetTagsEmpty(t *testing.T) {
	cfg := &config.LogsConfig{}
	source := sources.NewLogSource("", cfg)
	origin := NewOrigin(source)
	origin.SetTags([]string{})
	assert.Equal(t, []string{}, origin.Tags(nil))
	assert.Equal(t, "", origin.TagsToString(nil))
	assert.Equal(t, []byte{}, origin.TagsPayload(nil))
}

func TestTagsWithConfigTagsOnly(t *testing.T) {
	cfg := &config.LogsConfig{
		Source:         "a",
		SourceCategory: "b",
		Tags:           []string{"c:d", "e"},
	}
	source := sources.NewLogSource("", cfg)
	origin := NewOrigin(source)
	assert.Equal(t, []string{"sourcecategory:b", "c:d", "e"}, origin.Tags(nil))
	assert.Equal(t, "[dd ddsource=\"a\"][dd ddsourcecategory=\"b\"][dd ddtags=\"c:d,e\"]", string(origin.TagsPayload(nil)))
	assert.Equal(t, "sourcecategory:b,c:d,e", origin.TagsToString(nil))
}

func TestSetTagsWithNoConfigTags(t *testing.T) {
	cfg := &config.LogsConfig{
		Source:         "a",
		SourceCategory: "b",
	}
	source := sources.NewLogSource("", cfg)
	origin := NewOrigin(source)
	origin.SetTags([]string{"foo:bar", "baz"})
	assert.Equal(t, []string{"foo:bar", "baz", "sourcecategory:b"}, origin.Tags(nil))
	assert.Equal(t, "foo:bar,baz,sourcecategory:b", origin.TagsToString(nil))
	assert.Equal(t, "[dd ddsource=\"a\"][dd ddsourcecategory=\"b\"][dd ddtags=\"foo:bar,baz\"]", string(origin.TagsPayload(nil)))
}

func TestSetTagsWithConfigTags(t *testing.T) {
	cfg := &config.LogsConfig{
		Source:         "a",
		SourceCategory: "b",
		Tags:           []string{"c:d", "e"},
	}
	source := sources.NewLogSource("", cfg)
	origin := NewOrigin(source)
	origin.SetTags([]string{"foo:bar", "baz"})
	assert.Equal(t, []string{"foo:bar", "baz", "sourcecategory:b", "c:d", "e"}, origin.Tags(nil))
	assert.Equal(t, "foo:bar,baz,sourcecategory:b,c:d,e", origin.TagsToString(nil))
	assert.Equal(t, "[dd ddsource=\"a\"][dd ddsourcecategory=\"b\"][dd ddtags=\"c:d,e,foo:bar,baz\"]", string(origin.TagsPayload(nil)))
}

func TestSetTagsWithConfigTagsAndProcessingTags(t *testing.T) {
	cfg := &config.LogsConfig{
		Source:         "a",
		SourceCategory: "b",
		Tags:           []string{"c:d", "e"},
	}
	source := sources.NewLogSource("", cfg)
	origin := NewOrigin(source)
	origin.SetTags([]string{"foo:bar", "baz"})
	assert.Equal(t, []string{"foo:bar", "baz", "sourcecategory:b", "c:d", "e", "processing:tag", "second:tag"}, origin.Tags([]string{"processing:tag", "second:tag"}))
	assert.Equal(t, "foo:bar,baz,sourcecategory:b,c:d,e,processing:tag,second:tag", origin.TagsToString([]string{"processing:tag", "second:tag"}))
	assert.Equal(t, "[dd ddsource=\"a\"][dd ddsourcecategory=\"b\"][dd ddtags=\"c:d,e,foo:bar,baz,processing:tag,second:tag\"]", string(origin.TagsPayload([]string{"processing:tag", "second:tag"})))
}

func TestDefaultSourceValueIsSourceFromConfig(t *testing.T) {
	var cfg *config.LogsConfig
	var source *sources.LogSource
	var origin *Origin

	cfg = &config.LogsConfig{Source: "foo"}
	source = sources.NewLogSource("", cfg)
	origin = NewOrigin(source)
	assert.Equal(t, "foo", origin.Source())

	origin.SetSource("bar")
	assert.Equal(t, "foo", origin.Source())

	cfg = &config.LogsConfig{}
	source = sources.NewLogSource("", cfg)
	origin = NewOrigin(source)
	assert.Equal(t, "", origin.Source())

	origin.SetSource("bar")
	assert.Equal(t, "bar", origin.Source())
}

func TestDefaultServiceValueIsServiceFromConfig(t *testing.T) {
	var cfg *config.LogsConfig
	var source *sources.LogSource
	var origin *Origin

	cfg = &config.LogsConfig{Service: "foo"}
	source = sources.NewLogSource("", cfg)
	origin = NewOrigin(source)
	assert.Equal(t, "foo", origin.Service())

	origin.SetService("bar")
	assert.Equal(t, "foo", origin.Service())

	cfg = &config.LogsConfig{}
	source = sources.NewLogSource("", cfg)
	origin = NewOrigin(source)
	assert.Equal(t, "", origin.Service())

	origin.SetService("bar")
	assert.Equal(t, "bar", origin.Service())
}
