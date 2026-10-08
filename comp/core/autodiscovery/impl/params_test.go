// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package autodiscoveryimpl

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	autodiscovery "github.com/DataDog/datadog-agent/comp/core/autodiscovery/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	healthplatform "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestPreparationParams(t *testing.T) {
	for _, tc := range []struct {
		name   string
		params *autodiscovery.Params
	}{
		{name: "omitted defaults to lazy"},
		{name: "explicitly disabled", params: &autodiscovery.Params{}},
		{name: "enabled", params: &autodiscovery.Params{PreloadConfigsOnStart: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reqs := preparationTestRequires(t)
			var ac *AutoConfig
			var calls atomic.Int32
			opts := []fx.Option{
				fx.NopLogger,
				fxutil.FxLifecycleAdapter(),
				// Use the real constructor adapter to verify that the optional
				// Params dependency accepts both absence and fx.Supply.
				fxutil.ProvideComponentConstructor(NewComponent),
				fx.Supply(
					fx.Annotate(reqs.Config, fx.As(new(config.Component))),
					fx.Annotate(reqs.Log, fx.As(new(log.Component))),
					fx.Annotate(reqs.Secrets, fx.As(new(secrets.Component))),
					fx.Annotate(reqs.TaggerComp, fx.As(new(tagger.Component))),
					fx.Annotate(reqs.FilterStore, fx.As(new(workloadfilter.Component))),
					fx.Annotate(reqs.Telemetry, fx.As(new(telemetry.Component))),
					fx.Annotate(reqs.HealthPlatform, fx.As(new(healthplatform.Component))),
					reqs.WMeta,
				),
				fx.Invoke(func(c autodiscovery.Component) {
					ac = c.(*AutoConfig)
					ac.preparation.initialize = func() { calls.Add(1) }
				}),
			}
			if tc.params != nil {
				opts = append(opts, fx.Supply(*tc.params))
			}
			app := fx.New(opts...)
			require.NoError(t, app.Err())
			require.Nil(t, ac.preparation.done, "construction must not start preparation")
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			require.NoError(t, app.Start(ctx))
			t.Cleanup(func() { require.NoError(t, app.Stop(context.Background())) })
			if tc.params != nil && tc.params.PreloadConfigsOnStart {
				require.NotNil(t, ac.preparation.done, "startup must launch preparation when enabled")
			} else {
				require.Nil(t, ac.preparation.done, "startup must leave preparation lazy by default")
				require.Zero(t, calls.Load())
			}
			require.NoError(t, ac.LoadAndRun(ctx))
			require.EqualValues(t, 1, calls.Load(), "LoadAndRun must start or join the same preparation")
		})
	}
}
