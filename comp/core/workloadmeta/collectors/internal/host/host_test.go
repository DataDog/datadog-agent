// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package host implements the host tag Workloadmeta collector.
package host

import (
	"context"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
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
		fx.Supply(workloadmeta.NewParams()),
		fx.Supply(context.Background()),
		workloadmetafxmock.MockModuleV2(),
	))

	mockClock := clock.NewMock()
	c := collector{
		config: deps.Config,
		clock:  mockClock,
	}

	err := c.Start(ctx, deps.Wml)
	require.NoError(t, err)

	go func() { assertTagsAreInWorkloadmeta(t, deps.Wml, 10*time.Second, expectedTags) }()
	err = c.Pull(ctx)
	require.NoError(t, err)

	mockClock.Add(11 * time.Minute)
	mockClock.WaitForAllTimers()

	go func() { assertTagsAreInWorkloadmeta(t, deps.Wml, 10*time.Second, []string{}) }()
	err = c.Pull(ctx)
	require.NoError(t, err)
}

func assertTagsAreInWorkloadmeta(t *testing.T, wlmeta workloadmeta.Component, timeout time.Duration, expectedTags []string) {
	eventChan := wlmeta.Subscribe(
		"host-test",
		workloadmeta.NormalPriority,
		workloadmeta.NewFilterBuilder().AddKind(workloadmeta.KindHost).Build(),
	)
	defer wlmeta.Unsubscribe(eventChan)

	for {
		select {
		case eventBundle := <-eventChan:
			eventBundle.Acknowledge()

			// It's possible to receive an empty event bundle if the collector
			// didn't have time to run yet.
			if len(eventBundle.Events) == 0 {
				break
			}

			require.Len(t, eventBundle.Events, 1)
			require.ElementsMatch(t, expectedTags, eventBundle.Events[0].Entity.(*workloadmeta.HostTags).HostTags)
			return

		case <-time.After(timeout):
			require.Fail(t, "timed out waiting for event")
		}
	}
}
