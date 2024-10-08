// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package host implements the host tag Workloadmeta collector.
package host

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type testDeps struct {
	fx.In

	Config config.Component
	Wml    workloadmetamock.Mock
}

func TestHostCollector(t *testing.T) {
	expectedTags := []string{"tag1:value1", "tag2", "tag3"}
	ctx := context.TODO()

	overrides := map[string]interface{}{
		"tags":                   expectedTags,
		"expected_tags_duration": "10m",
	}

	deps := fxutil.Test[testDeps](t, fx.Options(
		fx.Replace(config.MockParams{Overrides: overrides}),
		core.MockBundle(),
		fx.Supply(context.Background()),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	mockClock := clock.NewMock()
	c := collector{
		config: deps.Config,
		clock:  mockClock,
	}

	err := c.Start(ctx, deps.Wml)
	require.NoError(t, err)

	expectedWorkloadmetaEvents := []workloadmeta.Event{
		{
			// Event generated by the first Pull() call
			Type: workloadmeta.EventTypeSet,
			Entity: &workloadmeta.HostTags{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindHost,
					ID:   "host",
				},
				HostTags: expectedTags,
			},
		},
		{
			// Event generated by the second Pull() call after more than
			// "config.expected_tags_duration" has passed
			Type: workloadmeta.EventTypeSet,
			Entity: &workloadmeta.HostTags{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindHost,
					ID:   "host",
				},
				HostTags: []string{},
			},
		},
	}

	// Create a subscriber in a different goroutine and check that it receives
	// the expected events
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		assertExpectedEventsAreReceived(t, deps.Wml, 10*time.Second, expectedWorkloadmetaEvents)
		wg.Done()
	}()

	err = c.Pull(ctx)
	require.NoError(t, err)

	mockClock.Add(11 * time.Minute) // Notice that this is more than the expected_tags_duration defined above
	mockClock.WaitForAllTimers()

	err = c.Pull(ctx)
	require.NoError(t, err)

	wg.Wait()
}

func assertExpectedEventsAreReceived(t *testing.T, wlmeta workloadmeta.Component, timeout time.Duration, expectedEvents []workloadmeta.Event) {
	eventChan := wlmeta.Subscribe(
		"host-test",
		workloadmeta.NormalPriority,
		workloadmeta.NewFilterBuilder().AddKind(workloadmeta.KindHost).Build(),
	)
	defer wlmeta.Unsubscribe(eventChan)

	var receivedEvents []workloadmeta.Event

	for len(receivedEvents) < len(expectedEvents) {
		select {
		case eventBundle := <-eventChan:
			eventBundle.Acknowledge()
			receivedEvents = append(receivedEvents, eventBundle.Events...)
		case <-time.After(timeout):
			require.Fail(t, "timed out waiting for event")
		}
	}

	assert.ElementsMatch(t, expectedEvents, receivedEvents)
}
