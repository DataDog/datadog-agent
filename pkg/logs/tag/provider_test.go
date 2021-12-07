// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tag

import (
	"sort"
	"testing"
	"time"

	coreConfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/require"
)

func TestProviderExpectedTags(t *testing.T) {
	m := coreConfig.Mock()
	clock := clock.NewMock()

	oldStartTime := coreConfig.StartTime
	coreConfig.StartTime = clock.Now()
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
			clock.Add(100 * time.Millisecond)
		}
	}

	sort.Strings(tags)
	sort.Strings(tt)
	require.Equal(t, tags, tt)

	// let the deadline expire
	clock.Add(expectedTagsDuration)
	// tags are now empty
	require.Empty(t, pp.GetTags())
}

func TestTHing(t *testing.T) {

	clock := clock.NewMock()
	proceed := make(chan struct{})
	go func() {
		// time.Sleep(300 * time.Millisecond) // make me fail
		clock.Sleep(1 * time.Second)
		close(proceed)
	}()

	// time.Sleep(300 * time.Millisecond) // make me pass
	clock.Add(time.Second)
	<-proceed
}
