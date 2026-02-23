// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package channel

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	config "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/schedulers"
)

func setup() (scheduler *Scheduler, spy *schedulers.MockSourceManager) {
	ch := make(chan *config.ChannelMessage)
	scheduler = NewScheduler("test source", "testy", ch)
	spy = &schedulers.MockSourceManager{}
	scheduler.Start(spy)
	return
}

func TestScheduler(t *testing.T) {
	s, spy := setup()

	require.Equal(t, len(spy.Events), 1)
	require.True(t, spy.Events[0].Add)
	source := spy.Events[0].Source
	assert.Equal(t, "test source", source.Name)
	assert.Equal(t, config.StringChannelType, source.Config.Type)
	assert.Equal(t, "testy", source.Config.Source)
	assert.Nil(t, source.Config.Tags)
	assert.Nil(t, source.Config.ChannelTags)

	s.SetLogsTags([]string{"foo"})

	require.Equal(t, len(spy.Events), 1) // no change
	assert.Nil(t, source.Config.Tags)
	assert.Equal(t, []string{"foo"}, []string(source.Config.ChannelTags))
}

func TestGetLogsTags(t *testing.T) {
	s, _ := setup()

	s.SetLogsTags([]string{"env:prod", "service:web"})
	tags := s.GetLogsTags()
	assert.Equal(t, []string{"env:prod", "service:web"}, tags)

	// Verify it's a defensive copy - modifying returned slice should not affect original
	tags[0] = "modified"
	assert.Equal(t, []string{"env:prod", "service:web"}, s.GetLogsTags())
}

func TestGetLogsTagsEmpty(t *testing.T) {
	s, _ := setup()
	s.SetLogsTags([]string{})
	tags := s.GetLogsTags()
	assert.Empty(t, tags)
}

func TestStop(t *testing.T) {
	s, _ := setup()
	// Stop should not panic
	s.Stop()
}

func TestSetSourceReplacesExisting(t *testing.T) {
	s, spy := setup()

	require.Len(t, spy.Events, 1)
	assert.True(t, spy.Events[0].Add)

	// Calling setSource again should remove the old and add a new source
	s.setSource()

	require.Len(t, spy.Events, 3)
	assert.False(t, spy.Events[1].Add) // Remove old
	assert.True(t, spy.Events[2].Add)  // Add new
}
