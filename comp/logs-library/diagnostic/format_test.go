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

// dropKeyFilter drops any tag whose key is in drop.
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

func formatTestMessage(t *testing.T) *message.Message {
	t.Helper()
	src := sources.NewLogSource("test", &config.LogsConfig{
		Type: config.FileType,
		Tags: []string{"env:prod"},
	})
	origin := message.NewOrigin(src)
	origin.SetTags([]string{"container_id:deadbeef", "team:infra"})
	return message.NewMessage([]byte("hello"), origin, message.StatusInfo, 0)
}

func tagsField(t *testing.T, formatted string) string {
	t.Helper()
	_, after, found := strings.Cut(formatted, "| Tags: ")
	assert.True(t, found, "formatted output has no Tags field: %s", formatted)
	before, _, _ := strings.Cut(after, " | Message:")
	return before
}

// TestFormatShowsTransportTags verifies stream-logs displays the outgoing tags.
func TestFormatShowsTransportTags(t *testing.T) {
	f := &logFormatter{hostname: getNewHostname("hostname")}
	msg := formatTestMessage(t)
	filter := &dropKeyFilter{drop: map[string]bool{"container_id": true}}
	msg.Origin.LogSource.SetTagFilterIfUnset(filter)

	got := tagsField(t, f.Format(msg, "", msg.GetContent()))

	assert.NotContains(t, got, "container_id")
	assert.Contains(t, got, "team:infra")
	assert.Contains(t, got, "env:prod")
	assert.NotEqual(t, msg.TagsToString(), got, "Tags() must stay unfiltered")
}
