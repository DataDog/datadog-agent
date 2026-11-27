// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package autodiscoveryimpl

import (
	"fmt"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/config"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	telemetrymock "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	admisconfig "github.com/DataDog/datadog-agent/comp/healthplatform/issues/ad-misconfiguration"
	healthplatformdef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	storefx "github.com/DataDog/datadog-agent/comp/healthplatform/store/fx"
	storeimpl "github.com/DataDog/datadog-agent/comp/healthplatform/store/impl"
	remotenoopfx "github.com/DataDog/datadog-agent/comp/healthplatform/store/remote/fx-noop"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// slowHealthPlatformSave is how long every save of the health platform store takes in
// TestTemplateFailuresDoNotHoldConfigLockOnSlowHealthPlatform: longer than the store's 10 s persist interval, so a
// save is still running when the next one is due
const slowHealthPlatformSave = 30 * time.Second

// TestTemplateFailuresDoNotHoldConfigLockOnSlowHealthPlatform reproduces the incident the asynchronous health platform
// store follows up on, on the Autodiscovery side: the config manager reports a template that fails to resolve, and
// resolves the issue of one that resolves, while holding its config lock, so a slow store stalled every Autodiscovery
// operation waiting for that lock. Against the real store, with every save taking slowHealthPlatformSave, processing a
// service must return without waiting, whether its template fails or resolves, and the failures must be reported.
//
// It runs in a testing/synctest bubble, where time only moves while every goroutine is blocked: processNewService
// holds the config lock for its whole run, so a run that waited on the store on a timer or a channel shows as elapsed
// time. A regression making it wait on a mutex held during the save would hang the test instead, as a goroutine
// blocked on a mutex keeps the bubble clock from advancing.
func TestTemplateFailuresDoNotHoldConfigLockOnSlowHealthPlatform(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		saves := storeimpl.SetSaveDelay(t, slowHealthPlatformSave)
		store := fxutil.Test[healthplatformdef.Component](t,
			storefx.Module(),
			remotenoopfx.Module(),
			fx.Provide(func() log.Component { return logmock.New(t) }),
			fx.Provide(func() config.Component {
				cfg := config.NewMock(t)
				cfg.SetInTest("health_platform.enabled", true)
				cfg.SetInTest("health_platform.persist_on_kubernetes", true)
				cfg.SetInTest("run_path", t.TempDir())
				return cfg
			}),
			telemetrymock.Module(),
			hostnameinterface.MockModule(),
		)
		// Registered after fxutil.Test, so it runs before the stop, also on a failure: let the stuck save finish and make
		// the final one fast, so the stop meets its deadline.
		t.Cleanup(func() {
			saves.SpeedUp()
			time.Sleep(slowHealthPlatformSave)
		})

		cm := newReconcilingConfigManager(&MockSecretResolver{}, store, nil, nil, nil, noopConfigPublisher).(*reconcilingConfigManager)
		// start and stop it like the Agent does: with Python, it runs a discovery worker that only stop shuts down
		cm.start()
		t.Cleanup(cm.stop)
		// %%extra_dbinstanceidentifier%% can't be resolved for the services below, so every one of them fails the
		// template and reports an issue
		tpl := integration.Config{
			Name:          "postgres",
			ADIdentifiers: []string{"postgres"},
			Instances:     []integration.Data{integration.Data("host: %%host%%\ntags:\n  - dbid:%%extra_dbinstanceidentifier%%")},
			Provider:      "file",
			Source:        "file:/etc/datadog-agent/conf.d/postgres.d/conf.yaml",
		}
		cm.processNewConfig(tpl)
		// a template every service resolves, which clears its issue on each resolution
		resolving := integration.Config{Name: "redis", ADIdentifiers: []string{"redis"}, LogsConfig: []byte("source: %%host%%")}
		cm.processNewConfig(resolving)

		const services = 35
		for i := range services {
			svc := &dummyServiceWithExtraConfigError{dummyService: dummyService{
				ID:            fmt.Sprintf("docker://%d", i),
				ADIdentifiers: []string{"postgres", "redis"},
				Hosts:         map[string]string{"main": "myhost"},
			}}
			start := time.Now()
			cm.processNewService(svc)
			require.Zero(t, time.Since(start), "processing %s held the config lock while the health platform was slow", svc.ID)
			time.Sleep(time.Second)
		}
		require.Positive(t, saves.Started(), "no slow save started while the services came in")

		synctest.Wait()
		for i := range services {
			issueID := fmt.Sprintf("%s:postgres:docker://%d:%s", admisconfig.TemplateIssueID, i, tpl.Digest())
			assert.NotNil(t, store.GetIssue(issueID), "the template failure of docker://%d was not reported", i)
		}
	})
}
