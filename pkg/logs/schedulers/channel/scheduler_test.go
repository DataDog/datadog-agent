// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package channel

import (
	"testing"

	config "github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/schedulers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setup() (scheduler *Scheduler, spy *schedulers.MockSourceManager) {
	ch := make(chan *config.ChannelMessage)
	scheduler = NewScheduler("test source", "testy", ch, nil)
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
	assert.Equal(t, []string{"foo"}, source.Config.ChannelTags)
}
