// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package aggregator exposes the AgentDemultiplexer as a DemultiplexerWithAggregator
package aggregator

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"go.uber.org/fx"
)

func getAggregator(agg *aggregator.AgentDemultiplexer) Component {
	return agg
}

func newMock(logger log.Component, lc fx.Lifecycle) Component {
	agg := aggregator.InitTestAgentDemultiplexerWithFlushInterval(logger, 10*time.Millisecond)
	lc.Append(fx.Hook{OnStop: func(context.Context) error {
		agg.Stop(false)
		return nil
	}})
	return agg
}
