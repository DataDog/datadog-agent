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

// tagParityCase pins the tag wire output of one Origin, as each tailer family
// wires it today. Expected values are literals: they are what the agent emits
// on main, and they must not change when the tags are served from a snapshot.
type tagParityCase struct {
	name           string
	source         string
	mappedSource   string
	sourceCategory string
	configTags     []string
	attached       []string // what the tailer passes to Origin.SetTags

	wantTags        []string
	wantTagsString  string
	wantPayload     string // TagsPayload(nil)
	wantPayloadProc string // TagsPayload([]string{"processing:tag", "second:tag"})
}

var parityProcessingTags = []string{"processing:tag", "second:tag"}

var tagParityCases = []tagParityCase{
	{
		name:            "file",
		source:          "nginx",
		sourceCategory:  "web",
		configTags:      []string{"env:prod", "team:infra"},
		attached:        []string{"filename:app.log", "dirname:/var/log", "container_name:nginx", "truncated:single_line"},
		wantTags:        []string{"filename:app.log", "dirname:/var/log", "container_name:nginx", "truncated:single_line", "sourcecategory:web", "env:prod", "team:infra"},
		wantTagsString:  "filename:app.log,dirname:/var/log,container_name:nginx,truncated:single_line,sourcecategory:web,env:prod,team:infra",
		wantPayload:     `[dd ddsource="nginx"][dd ddsourcecategory="web"][dd ddtags="env:prod,team:infra,filename:app.log,dirname:/var/log,container_name:nginx,truncated:single_line"]`,
		wantPayloadProc: `[dd ddsource="nginx"][dd ddsourcecategory="web"][dd ddtags="env:prod,team:infra,filename:app.log,dirname:/var/log,container_name:nginx,truncated:single_line,processing:tag,second:tag"]`,
	},
	{
		name:            "container",
		source:          "nginx",
		sourceCategory:  "web",
		configTags:      []string{"env:prod", "team:infra"},
		attached:        []string{"truncated:single_line", "noisy_log:true", "container_name:nginx", "image_name:nginx"},
		wantTags:        []string{"truncated:single_line", "noisy_log:true", "container_name:nginx", "image_name:nginx", "sourcecategory:web", "env:prod", "team:infra"},
		wantTagsString:  "truncated:single_line,noisy_log:true,container_name:nginx,image_name:nginx,sourcecategory:web,env:prod,team:infra",
		wantPayload:     `[dd ddsource="nginx"][dd ddsourcecategory="web"][dd ddtags="env:prod,team:infra,truncated:single_line,noisy_log:true,container_name:nginx,image_name:nginx"]`,
		wantPayloadProc: `[dd ddsource="nginx"][dd ddsourcecategory="web"][dd ddtags="env:prod,team:infra,truncated:single_line,noisy_log:true,container_name:nginx,image_name:nginx,processing:tag,second:tag"]`,
	},
	{
		name:            "socket",
		source:          "nginx",
		sourceCategory:  "web",
		configTags:      []string{"env:prod", "team:infra"},
		attached:        []string{"truncated:single_line"},
		wantTags:        []string{"truncated:single_line", "sourcecategory:web", "env:prod", "team:infra"},
		wantTagsString:  "truncated:single_line,sourcecategory:web,env:prod,team:infra",
		wantPayload:     `[dd ddsource="nginx"][dd ddsourcecategory="web"][dd ddtags="env:prod,team:infra,truncated:single_line"]`,
		wantPayloadProc: `[dd ddsource="nginx"][dd ddsourcecategory="web"][dd ddtags="env:prod,team:infra,truncated:single_line,processing:tag,second:tag"]`,
	},
	{
		// journald after PR0 (#55908): the provider is seeded empty, so the
		// configured tags are not part of what the tailer passes to SetTags.
		name:            "journald",
		source:          "nginx",
		sourceCategory:  "web",
		configTags:      []string{"env:prod", "team:infra"},
		attached:        []string{"container_name:nginx"},
		wantTags:        []string{"container_name:nginx", "sourcecategory:web", "env:prod", "team:infra"},
		wantTagsString:  "container_name:nginx,sourcecategory:web,env:prod,team:infra",
		wantPayload:     `[dd ddsource="nginx"][dd ddsourcecategory="web"][dd ddtags="env:prod,team:infra,container_name:nginx"]`,
		wantPayloadProc: `[dd ddsource="nginx"][dd ddsourcecategory="web"][dd ddtags="env:prod,team:infra,container_name:nginx,processing:tag,second:tag"]`,
	},
	{
		// windowsevent after PR0 (#55908): only the parsing tags reach SetTags.
		name:            "windowsevent",
		source:          "nginx",
		sourceCategory:  "web",
		configTags:      []string{"env:prod", "team:infra"},
		attached:        []string{"truncated:true"},
		wantTags:        []string{"truncated:true", "sourcecategory:web", "env:prod", "team:infra"},
		wantTagsString:  "truncated:true,sourcecategory:web,env:prod,team:infra",
		wantPayload:     `[dd ddsource="nginx"][dd ddsourcecategory="web"][dd ddtags="env:prod,team:infra,truncated:true"]`,
		wantPayloadProc: `[dd ddsource="nginx"][dd ddsourcecategory="web"][dd ddtags="env:prod,team:infra,truncated:true,processing:tag,second:tag"]`,
	},
	{
		name:            "everything empty",
		wantTags:        []string{},
		wantTagsString:  "",
		wantPayload:     ``,
		wantPayloadProc: `[dd ddtags="processing:tag,second:tag"]`,
	},
	{
		name:            "empty attached slice",
		attached:        []string{},
		wantTags:        []string{},
		wantTagsString:  "",
		wantPayload:     ``,
		wantPayloadProc: `[dd ddtags="processing:tag,second:tag"]`,
	},
	{
		name:            "source only",
		source:          "nginx",
		wantTags:        []string{},
		wantTagsString:  "",
		wantPayload:     `[dd ddsource="nginx"]`,
		wantPayloadProc: `[dd ddsource="nginx"][dd ddtags="processing:tag,second:tag"]`,
	},
	{
		name:            "source category only",
		sourceCategory:  "web",
		wantTags:        []string{"sourcecategory:web"},
		wantTagsString:  "sourcecategory:web",
		wantPayload:     `[dd ddsourcecategory="web"]`,
		wantPayloadProc: `[dd ddsourcecategory="web"][dd ddtags="processing:tag,second:tag"]`,
	},
	{
		name:            "configured tags only",
		configTags:      []string{"env:prod", "team:infra"},
		wantTags:        []string{"env:prod", "team:infra"},
		wantTagsString:  "env:prod,team:infra",
		wantPayload:     `[dd ddtags="env:prod,team:infra"]`,
		wantPayloadProc: `[dd ddtags="env:prod,team:infra,processing:tag,second:tag"]`,
	},
	{
		name:            "attached only",
		attached:        []string{"truncated:single_line"},
		wantTags:        []string{"truncated:single_line"},
		wantTagsString:  "truncated:single_line",
		wantPayload:     `[dd ddtags="truncated:single_line"]`,
		wantPayloadProc: `[dd ddtags="truncated:single_line,processing:tag,second:tag"]`,
	},
	{
		// A configured tag that is also attached appears twice, as today.
		name:            "duplicate configured and attached",
		source:          "nginx",
		configTags:      []string{"container_name:nginx"},
		attached:        []string{"container_name:nginx"},
		wantTags:        []string{"container_name:nginx", "container_name:nginx"},
		wantTagsString:  "container_name:nginx,container_name:nginx",
		wantPayload:     `[dd ddsource="nginx"][dd ddtags="container_name:nginx,container_name:nginx"]`,
		wantPayloadProc: `[dd ddsource="nginx"][dd ddtags="container_name:nginx,container_name:nginx,processing:tag,second:tag"]`,
	},
	{
		// remap_source runs in the processor, after the tailer.
		name:            "mapped source",
		source:          "nginx",
		mappedSource:    "nginx-remapped",
		sourceCategory:  "web",
		configTags:      []string{"env:prod"},
		attached:        []string{"truncated:single_line"},
		wantTags:        []string{"truncated:single_line", "sourcecategory:web", "env:prod"},
		wantTagsString:  "truncated:single_line,sourcecategory:web,env:prod",
		wantPayload:     `[dd ddsource="nginx-remapped"][dd ddsourcecategory="web"][dd ddtags="env:prod,truncated:single_line"]`,
		wantPayloadProc: `[dd ddsource="nginx-remapped"][dd ddsourcecategory="web"][dd ddtags="env:prod,truncated:single_line,processing:tag,second:tag"]`,
	},
}

// newParityOrigin wires an Origin the way a tailer does: a LogSource carrying
// the configured tags, then SetTags with the tailer-attached tags.
func newParityOrigin(c tagParityCase) *Origin {
	source := sources.NewLogSource("", &config.LogsConfig{
		Source:         c.source,
		SourceCategory: c.sourceCategory,
		Tags:           c.configTags,
	})
	origin := NewOrigin(source)
	if c.attached != nil {
		origin.SetTags(c.attached)
	}
	return origin
}

func assertTagParity(t *testing.T, c tagParityCase, origin *Origin) {
	t.Helper()
	assert.Equal(t, c.wantTags, origin.Tags(), "Tags()")
	assert.Equal(t, c.wantTagsString, origin.TagsToString(), "TagsToString()")
	assert.Equal(t, []byte(c.wantPayload), origin.TagsPayload(nil), "TagsPayload(nil)")
	assert.NotNil(t, origin.TagsPayload(nil), "TagsPayload(nil) is never nil")
	assert.Equal(t, []byte(c.wantPayloadProc), origin.TagsPayload(parityProcessingTags), "TagsPayload(processing)")
}

// TestTagWireParity pins the tag output of Origin for each tailer family's
// wiring. Any ordering flip, group-boundary shift or duplicated tag fails.
func TestTagWireParity(t *testing.T) {
	for _, c := range tagParityCases {
		t.Run(c.name, func(t *testing.T) {
			origin := newParityOrigin(c)
			if c.mappedSource != "" {
				origin.SetMappedSource(c.mappedSource)
			}
			assertTagParity(t, c, origin)
		})
	}
}

// TestTagWireParityNilOrigin pins the nil-origin and nil-source edges.
func TestTagWireParityNilOrigin(t *testing.T) {
	var nilOrigin *Origin
	assert.Nil(t, nilOrigin.Tags())
	assert.Equal(t, "", nilOrigin.TagsToString())
	assert.Equal(t, []byte{}, nilOrigin.TagsPayload(nil))

	noSource := &Origin{}
	noSource.SetTags([]string{"a:b"})
	assert.Nil(t, noSource.Tags())
	assert.Equal(t, "", noSource.TagsToString())
	assert.Equal(t, []byte{}, noSource.TagsPayload(parityProcessingTags))
}
