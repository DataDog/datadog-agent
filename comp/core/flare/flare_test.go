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
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/secrets/secretsimpl"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	taggermock "github.com/DataDog/datadog-agent/comp/core/tagger/mock"
	nooptelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/noopsimpl"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	rcclienttypes "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/types"

	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type rcSettings struct {
	duration         time.Duration
	blockRate        int
	mutexFrac        int
	enableStreamLogs bool
}

func getFlare(t *testing.T, overrides map[string]interface{}, fillers ...fx.Option) *flare {
	fillerModule := fxutil.Component(fillers...)
	fakeTagger := taggerfxmock.SetupFakeTagger(t)
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
			fx.Provide(func() taggermock.Mock { return fakeTagger }),
			fx.Provide(func() tagger.Component { return fakeTagger }),
			fillerModule,
			fx.Provide(func() ipc.Component { return ipcmock.New(t) }),
			fx.Provide(func(ipcComp ipc.Component) ipc.HTTPClient { return ipcComp.GetClient() }),
		),
	).Comp.(*flare)
}

// CreateFlareBuilderMockFactory generates a FlareBuilderFactory that will output mocked builders when called.
func setupMockBuilder(t *testing.T) func() {
	fbFactory = func(localFlare bool, flareArgs types.FlareArgs) (types.FlareBuilder, error) {
		return helpers.NewFlareBuilderMockWithArgs(t, localFlare, flareArgs), nil
	}

	return func() {
		fbFactory = helpers.NewFlareBuilder
	}
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

func TestAgentTaskFlareProfilingArgs(t *testing.T) {
	defer setupMockBuilder(t)()

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

	runFlareTestScenarios(t, testCfg, scenarios, func(fb types.FlareBuilder, expSettings rcSettings) {
		assert.Equal(t, expSettings.duration, fb.GetFlareArgs().ProfileDuration)
		assert.Equal(t, expSettings.blockRate, fb.GetFlareArgs().ProfileBlockingRate)
		assert.Equal(t, expSettings.mutexFrac, fb.GetFlareArgs().ProfileMutexFraction)
	})
}

func TestAgentTaskFlareStreamLogsArgs(t *testing.T) {
	defer setupMockBuilder(t)()

	enabledDefaults := rcSettings{
		enableStreamLogs: true,
		duration:         60 * time.Second,
	}

	disabledDefaults := rcSettings{
		enableStreamLogs: false,
	}

	testCfg := map[string]interface{}{
		"site":                         "localhost", // Provide extra guarantees we don't try to send the flare off the box
		"flare.rc_streamlogs.duration": enabledDefaults.duration,
	}

	scenarios := []struct {
		name        string
		task        string
		expSettings rcSettings
	}{
		{
			name:        "Test stream logs enabled",
			task:        "{\"args\":{\"case_id\":\"22420\",\"enable_streamlogs\":\"true\",\"user_handle\":\"no-reply@datadoghq.com\"},\"task_type\":\"flare\",\"uuid\":\"a_uuid\"}",
			expSettings: enabledDefaults,
		},
		{
			name:        "Test stream logs disabled",
			task:        "{\"args\":{\"case_id\":\"22420\",\"enable_streamlogs\":\"false\",\"user_handle\":\"no-reply@datadoghq.com\"},\"task_type\":\"flare\",\"uuid\":\"a_uuid\"}",
			expSettings: disabledDefaults,
		},
		{
			name:        "Test stream logs invalid",
			task:        "{\"args\":{\"case_id\":\"22420\",\"enable_streamlogs\":\"1\",\"user_handle\":\"no-reply@datadoghq.com\"},\"task_type\":\"flare\",\"uuid\":\"a_uuid\"}",
			expSettings: disabledDefaults,
		},
		{
			name:        "Test stream logs not present",
			task:        "{\"args\":{\"case_id\":\"22420\",\"user_handle\":\"no-reply@datadoghq.com\"},\"task_type\":\"flare\",\"uuid\":\"a_uuid\"}",
			expSettings: disabledDefaults,
		},
	}

	runFlareTestScenarios(t, testCfg, scenarios, func(fb types.FlareBuilder, expSettings rcSettings) {
		assert.Equal(t, expSettings.duration, fb.GetFlareArgs().StreamLogsDuration)
	})
}

func runFlareTestScenarios(t *testing.T, testCfg map[string]interface{}, scenarios []struct {
	name        string
	task        string
	expSettings rcSettings
}, assertFunc func(fb types.FlareBuilder, expSettings rcSettings)) {
	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			flare := getFlare(t, testCfg)

			flare.providers = []*types.FlareFiller{
				types.NewFiller(func(fb types.FlareBuilder) error {
					assertFunc(fb, s.expSettings)
					return nil
				}),
			}
			atc, err := rcclienttypes.ParseConfigAgentTask([]byte(s.task), state.Metadata{})
			assert.NoError(t, err)

			flare.onAgentTaskEvent(rcclienttypes.TaskFlare, atc)
		})
	}
}
