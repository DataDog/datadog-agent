// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package message

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/stretchr/testify/assert"
)

func TestSetTagsEmpty(t *testing.T) {
	cfg := &config.LogsConfig{}
	source := config.NewLogSource("", cfg)
	origin := NewOrigin(source)
	origin.SetTags([]string{})
	assert.Equal(t, []string{}, origin.Tags())
	assert.Equal(t, []byte{}, origin.TagsPayload())
}

func TestTagsWithConfigTagsOnly(t *testing.T) {
	cfg := &config.LogsConfig{
		Source:         "a",
		SourceCategory: "b",
		Tags:           []string{"c:d", "e"},
	}
	source := config.NewLogSource("", cfg)
	origin := NewOrigin(source)
	assert.Equal(t, []string{"source:a", "sourcecategory:b", "c:d", "e"}, origin.Tags())
	assert.Equal(t, "[dd ddsource=\"a\"][dd ddsourcecategory=\"b\"][dd ddtags=\"c:d,e\"]", string(origin.TagsPayload()))
}

func TestSetTagsWithNoConfigTags(t *testing.T) {
	cfg := &config.LogsConfig{
		Source:         "a",
		SourceCategory: "b",
	}
	source := config.NewLogSource("", cfg)
	origin := NewOrigin(source)
	origin.SetTags([]string{"foo:bar", "baz"})
	assert.Equal(t, []string{"foo:bar", "baz", "source:a", "sourcecategory:b"}, origin.Tags())
	assert.Equal(t, "[dd ddsource=\"a\"][dd ddsourcecategory=\"b\"][dd ddtags=\"foo:bar,baz\"]", string(origin.TagsPayload()))
}

func TestSetTagsWithConfigTags(t *testing.T) {
	cfg := &config.LogsConfig{
		Source:         "a",
		SourceCategory: "b",
		Tags:           []string{"c:d", "e"},
	}
	source := config.NewLogSource("", cfg)
	origin := NewOrigin(source)
	origin.SetTags([]string{"foo:bar", "baz"})
	assert.Equal(t, []string{"foo:bar", "baz", "source:a", "sourcecategory:b", "c:d", "e"}, origin.Tags())
	assert.Equal(t, "[dd ddsource=\"a\"][dd ddsourcecategory=\"b\"][dd ddtags=\"c:d,e,foo:bar,baz\"]", string(origin.TagsPayload()))
}
