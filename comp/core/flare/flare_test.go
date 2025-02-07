// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/demultiplexerimpl"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/autodiscoveryimpl"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/scheduler"
	"github.com/DataDog/datadog-agent/comp/core/config"
	flarebuilder "github.com/DataDog/datadog-agent/comp/core/flare/builder"
	"github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	"github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/secrets/secretsimpl"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	mockTagger "github.com/DataDog/datadog-agent/comp/core/tagger/mock"
	nooptelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/noopsimpl"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"

	rcclienttypes "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/types"

	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func getFlare(t *testing.T, overrides map[string]interface{}, fillers ...fx.Option) *flare {
	fillerModule := fxutil.Component(fillers...)
	fakeTagger := mockTagger.SetupFakeTagger(t)
	return newFlare(
		fxutil.Test[dependencies](
			t,
			fx.Provide(func() log.Component { return logmock.New(t) }),
			config.MockModule(),
			fx.Replace(config.MockParams{
				Overrides: overrides,
			}),
			secretsimpl.MockModule(),
			nooptelemetry.Module(),
			hostnameimpl.MockModule(),
			demultiplexerimpl.MockModule(),
			fx.Provide(func() Params { return Params{} }),
			collector.NoneModule(),
			workloadmetafxmock.MockModule(workloadmeta.NewParams()),
			autodiscoveryimpl.MockModule(),
			fx.Supply(autodiscoveryimpl.MockParams{Scheduler: scheduler.NewController()}),
			fx.Provide(func(ac autodiscovery.Mock) autodiscovery.Component { return ac.(autodiscovery.Component) }),
			fx.Provide(func() mockTagger.Mock { return fakeTagger }),
			fx.Provide(func() tagger.Component { return fakeTagger }),
			fx.Supply(helpers.CreateFlareBuilderMockFactory(t)),
			fillerModule,
		),
	).Comp.(*flare)
}
func TestFlareCreation(t *testing.T) {
	realProvider := types.NewFiller(func(_ types.FlareBuilder) error { return nil })

	flare := getFlare(
		t,
		map[string]interface{}{},
		// provider a nil FlareFiller
		fx.Provide(fx.Annotate(
			func() *types.FlareFiller { return nil },
			fx.ResultTags(`group:"flare"`),
		)),
		// provider a real FlareFiller
		fx.Provide(fx.Annotate(
			func() *types.FlareFiller { return realProvider },
			fx.ResultTags(`group:"flare"`),
		)),
	)

	assert.GreaterOrEqual(t, len(flare.providers), 1)
	assert.NotContains(t, flare.providers, nil)
}

func TestRunProviders(t *testing.T) {
	firstStarted := make(chan struct{}, 1)
	var secondDone atomic.Bool

	flare := getFlare(
		t,
		map[string]interface{}{},
		// provider a nil FlareFiller
		fx.Provide(fx.Annotate(
			func() *types.FlareFiller { return nil },
			fx.ResultTags(`group:"flare"`),
		)),
		fx.Provide(fx.Annotate(
			func() *types.FlareFiller {
				return types.NewFiller(func(_ types.FlareBuilder) error {
					firstStarted <- struct{}{}
					return nil
				})
			},
			fx.ResultTags(`group:"flare"`),
		)),
		fx.Provide(fx.Annotate(
			func() *types.FlareFiller {
				return types.NewFiller(func(_ types.FlareBuilder) error {
					time.Sleep(10 * time.Second)
					secondDone.Store(true)
					return nil
				})
			},
			fx.ResultTags(`group:"flare"`),
		)),
	)

	cliProviderTimeout := time.Nanosecond

	fb, err := helpers.NewFlareBuilder(false, flarebuilder.FlareArgs{})
	require.NoError(t, err)

	start := time.Now()
	flare.runProviders(fb, cliProviderTimeout)
	// ensure that providers are actually started
	<-firstStarted
	elapsed := time.Since(start)

	// ensure that we're not blocking for the slow provider
	assert.Less(t, elapsed, 5*time.Second)
	assert.False(t, secondDone.Load())
}

func TestAgentTaskFlareArgs(t *testing.T) {
	type rcSettings struct {
		duration  time.Duration
		blockRate int
		mutexFrac int
	}

	enabledDefaults := rcSettings{
		duration:  40 * time.Second,
		blockRate: 123,
		mutexFrac: 456,
	}

	disabledDefaults := rcSettings{}

	testCfg := map[string]interface{}{
		"site":                                "localhost", // Provide extra guarantees we don't try to send the flare off the box
		"flare.rc_profiling.profile_duration": enabledDefaults.duration,
		"flare.rc_profiling.blocking_rate":    enabledDefaults.blockRate,
		"flare.rc_profiling.mutex_fraction":   enabledDefaults.mutexFrac,
	}

	scenarios := []struct {
		name        string
		task        string
		expSettings rcSettings
	}{
		{
			name:        "Test profiling enabled",
			task:        "{\"args\":{\"case_id\":\"22420\",\"enable_profiling\":\"true\",\"user_handle\":\"no-reply@datadoghq.com\"},\"task_type\":\"flare\",\"uuid\":\"a_uuid\"}",
			expSettings: enabledDefaults,
		},
		{
			name:        "Test profiling disabled",
			task:        "{\"args\":{\"case_id\":\"22420\",\"enable_profiling\":\"false\",\"user_handle\":\"no-reply@datadoghq.com\"},\"task_type\":\"flare\",\"uuid\":\"a_uuid\"}",
			expSettings: disabledDefaults,
		},
		{
			name:        "Test profiling invalid",
			task:        "{\"args\":{\"case_id\":\"22420\",\"enable_profiling\":\"1\",\"user_handle\":\"no-reply@datadoghq.com\"},\"task_type\":\"flare\",\"uuid\":\"a_uuid\"}",
			expSettings: disabledDefaults,
		},
		{
			name:        "Test profiling not present",
			task:        "{\"args\":{\"case_id\":\"22420\",\"user_handle\":\"no-reply@datadoghq.com\"},\"task_type\":\"flare\",\"uuid\":\"a_uuid\"}",
			expSettings: disabledDefaults,
		},
	}

	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			flare := getFlare(t, testCfg)

			flare.providers = []*types.FlareFiller{
				types.NewFiller(func(fb types.FlareBuilder) error {
					assert.Equal(t, s.expSettings.duration, fb.GetFlareArgs().ProfileDuration)
					assert.Equal(t, s.expSettings.blockRate, fb.GetFlareArgs().ProfileBlockingRate)
					assert.Equal(t, s.expSettings.mutexFrac, fb.GetFlareArgs().ProfileMutexFraction)
					return nil
				}),
			}
			atc, err := rcclienttypes.ParseConfigAgentTask([]byte(s.task), state.Metadata{})
			assert.NoError(t, err)

			flare.onAgentTaskEvent(rcclienttypes.TaskFlare, atc)
		})
	}
}
