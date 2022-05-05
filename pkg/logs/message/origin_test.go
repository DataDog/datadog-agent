// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package message

import (
	"regexp"
	"strings"
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
	assert.Equal(t, "", origin.TagsToString())
	assert.Equal(t, []byte{}, origin.TagsPayload())
}

func TestSetTags(t *testing.T) {

	tests := []struct {
		name         string
		configTags   []string
		setTags      []string
		expectedTags []string
	}{
		{
			name: "Empty tags",
		},
		{
			name:         "Config tags only",
			configTags:   []string{"c:d", "e"},
			expectedTags: []string{"c:d", "e"},
		},
		{
			name:         "Set tags with no config tags",
			setTags:      []string{"foo:bar", "baz"},
			expectedTags: []string{"foo:bar", "baz"},
		},
		{
			name:         "Set tags with config tags",
			configTags:   []string{"c:d", "e"},
			setTags:      []string{"foo:bar", "baz"},
			expectedTags: []string{"c:d", "e", "foo:bar", "baz"},
		},
		{
			name:         "Set tags with duplicate config tags",
			configTags:   []string{"c:d", "e", "dupe:tag"},
			setTags:      []string{"foo:bar", "baz", "dupe:tag"},
			expectedTags: []string{"c:d", "e", "foo:bar", "baz", "dupe:tag"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Given a LogsConfig exists with configTags
			cfg := &config.LogsConfig{
				Source:         "a",
				SourceCategory: "b",
				Tags:           tc.configTags,
			}
			source := config.NewLogSource("", cfg)
			origin := NewOrigin(source)
			// When origin.SetTags is set to setTags
			origin.SetTags(tc.setTags)

			// Then origin.Tags match the expected tags with sourcecategory
			expectedTags := append(tc.expectedTags, "sourcecategory:b")
			assert.ElementsMatch(t, expectedTags, origin.Tags())
			// And origin.TagsToString match the expected tags with sourcecategory
			assert.ElementsMatch(t, expectedTags, strings.Split(origin.TagsToString(), ","))
			// And origin.TagsPayload() matches
			tagsPayload := string(origin.TagsPayload())
			if len(tc.expectedTags) == 0 { // Should have no tags in payload
				assert.Equal(t, "[dd ddsource=\"a\"][dd ddsourcecategory=\"b\"]", tagsPayload)
				return
			}
			assert.True(t, strings.HasPrefix(tagsPayload, "[dd ddsource=\"a\"][dd ddsourcecategory=\"b\"][dd ddtags=\""), "Payload did not have correct prefix", tagsPayload)
			assertDDTags(t, tc.expectedTags, tagsPayload)
		})
	}
}

func assertDDTags(t *testing.T, expectedTags []string, ddtags string) {
	if len(expectedTags) == 0 {
		return
	}
	r, _ := regexp.Compile("ddtags=\"(.*)\"]")
	submatch := r.FindStringSubmatch(ddtags)
	assert.NotEmpty(t, submatch)
	assert.ElementsMatch(t, expectedTags, strings.Split(submatch[1], ","))
}

func TestDefaultSourceValueIsSourceFromConfig(t *testing.T) {
	var cfg *config.LogsConfig
	var source *config.LogSource
	var origin *Origin

	cfg = &config.LogsConfig{Source: "foo"}
	source = config.NewLogSource("", cfg)
	origin = NewOrigin(source)
	assert.Equal(t, "foo", origin.Source())

	origin.SetSource("bar")
	assert.Equal(t, "foo", origin.Source())

	cfg = &config.LogsConfig{}
	source = config.NewLogSource("", cfg)
	origin = NewOrigin(source)
	assert.Equal(t, "", origin.Source())

	origin.SetSource("bar")
	assert.Equal(t, "bar", origin.Source())
}

func TestDefaultServiceValueIsServiceFromConfig(t *testing.T) {
	var cfg *config.LogsConfig
	var source *config.LogSource
	var origin *Origin

	cfg = &config.LogsConfig{Service: "foo"}
	source = config.NewLogSource("", cfg)
	origin = NewOrigin(source)
	assert.Equal(t, "foo", origin.Service())

	origin.SetService("bar")
	assert.Equal(t, "foo", origin.Service())

	cfg = &config.LogsConfig{}
	source = config.NewLogSource("", cfg)
	origin = NewOrigin(source)
	assert.Equal(t, "", origin.Service())

	origin.SetService("bar")
	assert.Equal(t, "bar", origin.Service())
}
