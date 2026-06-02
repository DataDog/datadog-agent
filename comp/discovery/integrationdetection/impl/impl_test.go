// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// The test build tag is required because this file references config.NewMock
// and compdef.NewTestLifecycle, which are only compiled under //go:build test.
// Use `dda inv test` or `go test -tags test` to run these tests.
//
//go:build test

package integrationdetectionimpl

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/scheduler"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

// noopAD is a minimal autodiscovery.Component for tests that don't exercise
// the AD scheduling path.
type noopAD struct{}

func (noopAD) AddConfigProvider(types.ConfigProvider, bool, time.Duration) {}
func (noopAD) LoadAndRun(context.Context)                                   {}
func (noopAD) GetAllConfigs() []integration.Config                          { return nil }
func (noopAD) GetUnresolvedConfigs() []integration.Config                   { return nil }
func (noopAD) AddListeners([]pkgconfigsetup.Listeners)                      {}
func (noopAD) AddScheduler(string, scheduler.Scheduler, bool)               {}
func (noopAD) RemoveScheduler(string)                                       {}
func (noopAD) GetIDOfCheckWithEncryptedSecrets(id checkid.ID) checkid.ID    { return id }
func (noopAD) GetAutodiscoveryErrors() map[string]map[string]types.ErrorMsgSet {
	return nil
}
func (noopAD) AddConfigProviderFromCatalog(pkgconfigsetup.ConfigurationProviders) error { return nil }
func (noopAD) GetTelemetryStore() *telemetry.Store                                      { return nil }
func (noopAD) GetConfigCheck() integration.ConfigCheckResponse {
	return integration.ConfigCheckResponse{}
}

var _ autodiscovery.Component = noopAD{}

func requiresWithConfig(t *testing.T, enabled bool) (Requires, *compdef.TestLifecycle) {
	t.Helper()
	cfg := config.NewMock(t)
	cfg.SetWithoutSource(enabledKey, enabled)
	lc := compdef.NewTestLifecycle(t)
	return Requires{
		Lifecycle: lc,
		Config:    cfg,
		Log:       logmock.New(t),
		AC:        noopAD{},
	}, lc
}

func TestNewComponent_DisabledByConfig(t *testing.T) {
	reqs, _ := requiresWithConfig(t, false) // lifecycle not needed: component is disabled
	p, err := NewComponent(reqs)
	require.NoError(t, err)
	_, present := p.Comp.Get()
	assert.False(t, present, "component must be absent when disabled")
}

func TestNewComponent_EnabledByConfig(t *testing.T) {
	reqs, lc := requiresWithConfig(t, true)
	p, err := NewComponent(reqs)
	require.NoError(t, err)
	comp, present := p.Comp.Get()
	assert.True(t, present, "component must be present when enabled")

	ctx := context.Background()
	require.NoError(t, lc.Start(ctx))

	// The component must be callable after lifecycle start without panicking;
	// no integrations are scheduled in this unit test, so the result is nil.
	assert.Nil(t, comp.EnabledIntegrations())

	require.NoError(t, lc.Stop(ctx))
}
