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

	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

func TestProviderExpectedTags(t *testing.T) {
	m := configmock.New(t)
	clock := clock.NewMock()
	fakeTagger := taggerimpl.SetupFakeTagger(t)

	oldStartTime := pkgconfigsetup.StartTime
	then := clock.Now()
	pkgconfigsetup.StartTime = then
	defer func() {
		pkgconfigsetup.StartTime = oldStartTime
	}()

	tags := []string{"tag1:value1", "tag2", "tag3"}
	m.SetWithoutSource("tags", tags)
	defer m.SetWithoutSource("tags", nil)

	m.SetWithoutSource("logs_config.tagger_warmup_duration", "2")

	expectedTagsDuration := 5 * time.Second
	m.SetWithoutSource("logs_config.expected_tags_duration", "5s")
	defer m.SetWithoutSource("logs_config.expected_tags_duration", 0)

	p := newProviderWithClock(types.NewEntityID(types.ContainerID, "foo"), clock, fakeTagger)
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
