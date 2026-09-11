// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package streamlogs

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// TestFormatShowsTransportTags pins the Tags field to the tag set the intake
// actually receives, so a working tag filter does not look broken in
// `agent stream-logs`.
func TestFormatShowsTransportTags(t *testing.T) {
	source := sources.NewLogSource("test", &config.LogsConfig{
		Type:       "a",
		Source:     "b",
		Service:    "service_a",
		TagFilters: &config.TagFilters{Exclude: []string{"region:*"}},
	})
	msg := message.NewMessage([]byte("a"), message.NewOrigin(source), message.StatusInfo, 0)
	msg.Origin.SetTags([]string{"env:prod", "region:us-east-1"})

	line := Formatter{}.Format(msg, "", []byte("redacted"))

	assert.Contains(t, line, "| Tags: env:prod |")
	assert.NotContains(t, line, "region:us-east-1")
}
