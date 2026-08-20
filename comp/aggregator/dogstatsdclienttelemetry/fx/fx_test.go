// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package fx

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	dogstatsdclientdropdetector "github.com/DataDog/datadog-agent/comp/aggregator/dogstatsdclientdropdetector/def"
	telemetrymock "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type noopDropDetector struct{}

func (*noopDropDetector) ObserveClientBytes(dogstatsdclientdropdetector.ClientByteMetric, float64) {}
func (*noopDropDetector) CompleteFinalDogStatsDSerieFlush()                                        {}

type dependencies struct {
	fx.In

	Observers []aggregator.FinalDogStatsDSerieObserver `group:"dogstatsd_final_serie_observers"`
}

func TestModuleProvidesFinalDogStatsDSerieObserver(t *testing.T) {
	deps := fxutil.Test[dependencies](t,
		Module(),
		telemetrymock.Module(),
		fx.Provide(func() dogstatsdclientdropdetector.Component { return &noopDropDetector{} }),
	)

	require.Len(t, deps.Observers, 1)
}
