// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trace

import (
	"context"
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/secrets/secretsimpl"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafx "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/statsd"
	traceagentdef "github.com/DataDog/datadog-agent/comp/trace/agent/def"
	traceagentimpl "github.com/DataDog/datadog-agent/comp/trace/agent/impl"
	"github.com/DataDog/datadog-agent/comp/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: agent-apm

func TestBundleDependencies(t *testing.T) {
	fxutil.TestBundle(t, Bundle(),
		fx.Provide(func() context.Context { return context.TODO() }), // fx.Supply(ctx) fails with a missing type error.
		fx.Supply(core.BundleParams{}),
		core.Bundle(),
		fx.Supply(workloadmeta.NewParams()),
		workloadmetafx.Module(),
		statsd.Module(),
		fx.Provide(func(cfg config.Component) telemetry.TelemetryCollector { return telemetry.NewCollector(cfg.Object()) }),
		secretsimpl.MockModule(),
		fx.Supply(tagger.NewFakeTaggerParams()),
		taggerimpl.Module(),
		fx.Supply(&traceagentimpl.Params{}),
	)
}

func TestMockBundleDependencies(t *testing.T) {
	os.Setenv("DD_APP_KEY", "abc1234")
	defer func() { os.Unsetenv("DD_APP_KEY") }()

	os.Setenv("DD_DD_URL", "https://example.com")
	defer func() { os.Unsetenv("DD_DD_URL") }()

	// Only for test purposes to avoid setting a different default value.
	os.Setenv("DDTEST_DEFAULT_LOG_FILE_PATH", config.DefaultLogFilePath)
	defer func() { os.Unsetenv("DDTEST_DEFAULT_LOG_FILE_PATH") }()

	cfg := fxutil.Test[config.Component](t, fx.Options(
		fx.Provide(func() context.Context { return context.TODO() }), // fx.Supply(ctx) fails with a missing type error.
		fx.Supply(core.BundleParams{}),
		traceMockBundle,
		fx.Supply(workloadmeta.NewParams()),
		workloadmetafx.Module(),
		fx.Invoke(func(_ config.Component) {}),
		fx.Provide(func(cfg config.Component) telemetry.TelemetryCollector { return telemetry.NewCollector(cfg.Object()) }),
		statsd.MockModule(),
		fx.Supply(&traceagentimpl.Params{}),
		fx.Invoke(func(_ traceagentdef.Component) {}),
		MockBundle(),
		taggerimpl.Module(),
		fx.Supply(tagger.NewTaggerParams()),
	))

	require.NotNil(t, cfg.Object())
}

var traceMockBundle = core.MakeMockBundle(
	fx.Provide(func() logimpl.Params {
		return logimpl.ForDaemon("TRACE", "apm_config.log_file", config.DefaultLogFilePath)
	}),
	logimpl.TraceMockModule(),
)
