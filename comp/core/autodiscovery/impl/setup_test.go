// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package autodiscoveryimpl

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers"
	providertypes "github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/scheduler"
	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	secretsmock "github.com/DataDog/datadog-agent/comp/core/secrets/mock"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	telemetrymock "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	workloadfilterfxmock "github.com/DataDog/datadog-agent/comp/core/workloadfilter/fx-mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	hpnoopimpl "github.com/DataDog/datadog-agent/comp/healthplatform/store/noop-impl"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

func preparationTestRequires(t *testing.T) Requires {
	t.Helper()
	// Build the mocks that install their own config first, then install the
	// config for this AutoConfig instance (some providers use the global config).
	tagger := taggerfxmock.SetupFakeTagger(t)
	filter := workloadfilterfxmock.SetupMockFilter(t)
	cfg := config.NewMockWithOverrides(t, map[string]interface{}{
		"autoconfig_from_environment": false,
		// This suite exercises provider setup, not secret resolution. The shared
		// secrets mock does not handle absent metrics/logs config sections.
		"secret_backend_skip_checks":  true,
		"confd_path":                  t.TempDir(),
		"container_image.enabled":     false,
		"container_lifecycle.enabled": false,
		"sbom.enabled":                false,
	})
	return Requires{
		Config: cfg, Log: logmock.New(t), Secrets: secretsmock.New(t),
		TaggerComp: tagger, FilterStore: filter, Telemetry: telemetrymock.New(t),
		WMeta: option.None[workloadmeta.Component](), HealthPlatform: hpnoopimpl.NewNoopComponent(),
	}
}

func startPreparationTestComponent(t *testing.T, reqs Requires) *AutoConfig {
	t.Helper()
	lc := compdef.NewTestLifecycle(t)
	reqs.Lc = lc
	ac := NewComponent(reqs).Comp.(*AutoConfig)
	require.NoError(t, lc.Start(t.Context()))
	t.Cleanup(func() { require.NoError(t, lc.Stop(context.Background())) })
	return ac
}

type preparationTestProvider struct {
	collects atomic.Int32
}

func (*preparationTestProvider) String() string { return "manual-test" }
func (*preparationTestProvider) GetConfigErrors() map[string]providertypes.ErrorMsgSet {
	return nil
}
func (*preparationTestProvider) IsUpToDate(context.Context) (bool, error) { return true, nil }
func (p *preparationTestProvider) Collect(context.Context) ([]integration.Config, error) {
	p.collects.Add(1)
	return nil, nil
}

type preparationTestScheduler struct {
	scheduled chan struct{}
}

func (s *preparationTestScheduler) Schedule(configs []integration.Config) {
	for _, cfg := range configs {
		if cfg.Name == "preparation_test" {
			s.scheduled <- struct{}{}
		}
	}
}
func (*preparationTestScheduler) Unschedule([]integration.Config) {}
func (*preparationTestScheduler) Stop()                           {}

func TestLoadAndRunDefaultAndManualProviders(t *testing.T) {
	for _, preload := range []bool{false, true} {
		t.Run(map[bool]string{false: "lazy", true: "preloaded"}[preload], func(t *testing.T) {
			reqs := preparationTestRequires(t)
			reqs.Params.PreloadConfigsOnStart = preload
			dir := reqs.Config.GetString("confd_path")
			require.NoError(t, os.WriteFile(filepath.Join(dir, "preparation_test.yaml"), []byte("init_config: {}\ninstances:\n  - value: test\n"), 0600))
			providers.ResetReader([]string{dir})
			t.Cleanup(func() { providers.ResetReader(nil) })
			ac := startPreparationTestComponent(t, reqs)
			if preload {
				require.NotNil(t, ac.preparation.done)
			} else {
				// Without opting in, startup must not configure any defaults.
				require.Nil(t, ac.preparation.done)
				require.Empty(t, ac.getConfigPollers())
			}

			scheduled := make(chan struct{}, 1)
			ac.AddScheduler("preparation-test", &preparationTestScheduler{scheduled: scheduled}, true)
			manual := &preparationTestProvider{}
			ac.AddConfigProvider(manual, false, 0)
			require.NoError(t, ac.LoadAndRun(t.Context()))
			require.EqualValues(t, 1, manual.collects.Load())
			require.Len(t, ac.getConfigPollers(), 2, "manual registration must neither suppress nor duplicate defaults")
			configs := ac.GetAllConfigs()
			require.Len(t, configs, 1)
			require.Equal(t, "preparation_test", configs[0].Name)
			require.Equal(t, "file", configs[0].Provider)
			// Observe delivery before tearing down the asynchronous scheduler.
			select {
			case <-scheduled:
			case <-time.After(5 * time.Second):
				t.Fatal("file config was not scheduled")
			}
		})
	}
}

func TestLoadAndRunDoesNotStartProvidersBeforePreparation(t *testing.T) {
	ac := startPreparationTestComponent(t, preparationTestRequires(t))
	release := make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	t.Cleanup(unblock) // Release preparation before the component's cleanup joins it.
	prepared := &preparationTestProvider{}
	ac.preparation.initialize = func() {
		<-release
		ac.AddConfigProvider(prepared, false, 0)
	}
	manual := &preparationTestProvider{}
	ac.AddConfigProvider(manual, false, 0)
	_, _, err := ac.preparation.start(t.Context())
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, ac.LoadAndRun(ctx), context.Canceled)
	require.Zero(t, manual.collects.Load())
	require.Zero(t, prepared.collects.Load())
	unblock()
	require.NoError(t, ac.LoadAndRun(t.Context()))
	require.EqualValues(t, 1, manual.collects.Load())
	require.EqualValues(t, 1, prepared.collects.Load())
}

func TestLoadAndRunBareInstance(t *testing.T) {
	reqs := preparationTestRequires(t)
	ac := NewAutoConfigFromDeps(scheduler.NewControllerAndStart(), reqs.Secrets, reqs.WMeta, reqs.TaggerComp, reqs.Log, reqs.Telemetry, reqs.FilterStore, reqs.HealthPlatform)
	ac.start()
	t.Cleanup(ac.stop)
	manual := &preparationTestProvider{}
	ac.AddConfigProvider(manual, false, 0)
	require.Nil(t, ac.preparation, "the low-level mock constructor must not install default preparation")
	require.NoError(t, ac.LoadAndRun(t.Context()))
	require.EqualValues(t, 1, manual.collects.Load())
	require.Len(t, ac.getConfigPollers(), 1)
	require.Empty(t, ac.listeners)
}
