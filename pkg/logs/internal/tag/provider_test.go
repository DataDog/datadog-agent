// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tag

import (
	"sort"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/require"

	coreConfig "github.com/DataDog/datadog-agent/pkg/config"
)

func TestProviderExpectedTags(t *testing.T) {
	m := coreConfig.Mock(t)
	clock := clock.NewMock()

	oldStartTime := coreConfig.StartTime
	then := clock.Now()
	coreConfig.StartTime = then
	defer func() {
		coreConfig.StartTime = oldStartTime
	}()

	tags := []string{"tag1:value1", "tag2", "tag3"}
	m.Set("tags", tags)
	defer m.Set("tags", nil)

	m.Set("logs_config.tagger_warmup_duration", "2")

	expectedTagsDuration := 5 * time.Second
	m.Set("logs_config.expected_tags_duration", "5s")
	defer m.Set("logs_config.expected_tags_duration", 0)

	p := newProviderWithClock("foo", clock)
	pp := p.(*provider)

	var tt []string

	// this will block for two (mock) seconds, so do it in a goroutine
	tagsChan := make(chan []string)
	go func() {
		tagsChan <- pp.GetTags()
	}()

wait:
	for {
		select {
		case tt = <-tagsChan:
			break wait
		default:
			clock.Add(90 * time.Millisecond)
		}
	}

	// Ensure we waited at least 2 seconds
	require.True(t, clock.Now().After(then.Add(2*time.Second)))

	sort.Strings(tags)
	sort.Strings(tt)
	require.Equal(t, tags, tt)

	// let the deadline expire
	clock.Add(expectedTagsDuration)

	// tags are now empty
	require.Empty(t, pp.GetTags())
}
