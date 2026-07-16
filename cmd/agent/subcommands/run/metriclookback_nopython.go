// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !python && !test

package run

import (
	"context"

	"go.uber.org/fx"

	metriclookbackdef "github.com/DataDog/datadog-agent/comp/metriclookback/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
)

type unavailableMetricLookback struct{}

func (unavailableMetricLookback) NewSenderManager(context.Context, string) sender.SenderManager {
	return nil
}

func metriclookbackModule() fx.Option {
	// Keep concrete metric lookback retention out of non-Python Agent binaries,
	// while satisfying shared Agent startup wiring with an unavailable component.
	return fx.Provide(func() metriclookbackdef.Component {
		return unavailableMetricLookback{}
	})
}
