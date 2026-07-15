// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package hostprofiler

import (
	"testing"

	config "github.com/DataDog/datadog-agent/comp/core/config"
	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	secretsmock "github.com/DataDog/datadog-agent/comp/core/secrets/mock"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	taggermock "github.com/DataDog/datadog-agent/comp/core/tagger/mock"
	telemetrymock "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	collectorimpl "github.com/DataDog/datadog-agent/comp/host-profiler/collector/impl"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	serializermock "github.com/DataDog/datadog-agent/pkg/serializer/mocks"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// TestBundleDependencies tests that the bundle can be created without errors.
//
// This test ensures that all dependencies in the host profiler bundle are
// properly configured and can be resolved by the dependency injection container.
func TestBundleDependencies(t *testing.T) {
	fxutil.TestBundle(t,
		Bundle(collectorimpl.NewParams("", false)),
		fx.Provide(collectorimpl.NewExtraFactoriesWithoutAgentCore),
		// dogtelextension (wired into GetExtensions in standalone mode) needs its own local
		// workloadmeta+tagger+hostname+secrets+telemetry+serializer; supply mocks for each.
		fx.Provide(func(t testing.TB) config.Component { return config.NewMock(t) }),
		fx.Provide(func(t testing.TB) serializer.MetricSerializer { return serializermock.NewMetricSerializer(t) }),
		fx.Provide(logmock.New),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
		taggerfxmock.MockModule(),
		fx.Provide(func(m taggermock.Mock) tagger.Component { return m }),
		hostnamemock.MockModule(),
		fx.Provide(func(t testing.TB) secrets.Component { return secretsmock.New(t) }),
		telemetrymock.Module(),
	)
}
