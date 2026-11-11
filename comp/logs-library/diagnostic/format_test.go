// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package diagnostic

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

type dropKeyFilter struct {
	drop map[string]bool
}

func (f *dropKeyFilter) Keep(tags []string) []string {
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		if key, _, _ := strings.Cut(t, ":"); !f.drop[key] {
			out = append(out, t)
		}
	}
	return out
}

func (f *dropKeyFilter) RetainsTag(key, _ string) bool {
	return !f.drop[key]
}

func TestFormatShowsTransportTags(t *testing.T) {
	f := &logFormatter{hostname: getNewHostname("hostname")}
	src := sources.NewLogSource("test", &config.LogsConfig{Type: config.FileType, Tags: []string{"env:prod"}})
	origin := message.NewOrigin(src)
	origin.SetTags([]string{"container_id:deadbeef", "team:infra"})
	msg := message.NewMessage([]byte("hello"), origin, message.StatusInfo, 0)
	filter := &dropKeyFilter{drop: map[string]bool{"container_id": true}}
	msg.Origin.LogSource.SetTagFilterIfUnset(filter)

	_, after, found := strings.Cut(f.Format(msg, "", msg.GetContent()), "| Tags: ")
	assert.True(t, found)
	got, _, _ := strings.Cut(after, " | Message:")

	assert.NotContains(t, got, "container_id")
	assert.Contains(t, got, "team:infra")
	assert.Contains(t, got, "env:prod")
	assert.NotEqual(t, msg.TagsToString(), got, "Tags() must stay unfiltered")
}
