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
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	"go.uber.org/fx"
)

type testDeps struct {
	fx.In

	Config config.Component
	Wml    workloadmeta.Mock
}

func TestHostCollector(t *testing.T) {
	expectedTags := []string{"tag1:value1", "tag2", "tag3"}

	overrides := map[string]interface{}{
		"tags":                   expectedTags,
		"expected_tags_duration": "10m",
	}

	deps := fxutil.Test[testDeps](t, fx.Options(
		fx.Replace(config.MockParams{Overrides: overrides}),
		core.MockBundle(),
		fx.Supply(workloadmeta.NewParams()),
		fx.Supply(context.Background()),
		workloadmeta.MockModule(),
	))

	eventChan := deps.Wml.SubscribeToEvents()

	mockClock := clock.NewMock()
	c := collector{
		config: deps.Config,
		clock:  mockClock,
	}

	c.Start(context.TODO(), deps.Wml)

	assertTags(t, (<-eventChan).Entity, expectedTags)

	mockClock.Add(11 * time.Minute)
	mockClock.WaitForAllTimers()

	assertTags(t, (<-eventChan).Entity, []string{})
}

func assertTags(t *testing.T, entity workloadmeta.Entity, expectedTags []string) {
	e := entity.(*workloadmeta.HostTags)
	assert.ElementsMatch(t, e.HostTags, expectedTags)
}
