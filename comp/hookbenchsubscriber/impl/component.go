// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package hookbenchsubscriberimpl implements the hookbenchsubscriber component.
package hookbenchsubscriberimpl

import (
	"context"
	"fmt"

	"go.uber.org/fx"

	hookbenchsubscriber "github.com/DataDog/datadog-agent/comp/hookbenchsubscriber/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/hook"
)

// Requires defines the dependencies for the hookbenchsubscriber component.
type Requires struct {
	fx.In

	Lc          fx.Lifecycle
	Config      config.Component
	MetricHooks []hook.Hook[[]hook.MetricSampleSnapshot] `group:"hook"`
}

// Provides defines the output of the hookbenchsubscriber component.
type Provides struct {
	fx.Out

	Comp hookbenchsubscriber.Component
}

type component struct{}

// NewComponent creates a new hookbenchsubscriber component.
// It registers hook.bench_subscriber_count no-op subscribers on every metrics
// pipeline hook. Each callback discards the payload immediately with no
// allocation, so only the hook delivery mechanism itself is measured.
func NewComponent(reqs Requires) Provides {
	n := reqs.Config.GetInt("hook.bench_subscriber_count")
	var unsubs []func()
	for _, h := range reqs.MetricHooks {
		for i := range n {
			unsub := h.Subscribe(
				fmt.Sprintf("bench-%d", i),
				func(_ []hook.MetricSampleSnapshot) {},
			)
			unsubs = append(unsubs, unsub)
		}
	}
	reqs.Lc.Append(fx.Hook{
		OnStop: func(_ context.Context) error {
			for _, u := range unsubs {
				u()
			}
			return nil
		},
	})
	return Provides{Comp: &component{}}
}
