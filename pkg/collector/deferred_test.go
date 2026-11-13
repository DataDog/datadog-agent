// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collector

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/loaders"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

// pendingLoader behaves like the python loader waiting on a dependency:
// until ready is set it returns a DepsPendingError, afterwards it loads.
type pendingLoader struct {
	mu    sync.Mutex
	ready bool
	deps  []string
}

func (l *pendingLoader) Name() string { return "python" }

func (l *pendingLoader) Load(_ sender.SenderManager, config integration.Config, _ integration.Data, _ int) (check.Check, error) {
	l.mu.Lock()
	ready := l.ready
	l.mu.Unlock()
	if !ready {
		return nil, loaders.NewDepsPendingError(l.deps)
	}
	mockCheck := MockCheck{Name: config.Name, LoaderName: l.Name(), CheckID: checkid.ID(config.Name)}
	return &mockCheck, nil
}

// DeclaredDeps implements the depsDeclarer interface, like the python loader
// backed by the dependency manifest.
func (l *pendingLoader) DeclaredDeps(_ integration.Config) []string {
	return l.deps
}

func (l *pendingLoader) setReady(ready bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.ready = ready
}

// fakeDepInstaller records the install and remove requests.
type fakeDepInstaller struct {
	mu     sync.Mutex
	inst   int
	remove int
}

func (f *fakeDepInstaller) InstallDepPackage(_, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inst++
	return nil
}

func (f *fakeDepInstaller) RemoveDepPackage(_ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.remove++
	return nil
}

func (f *fakeDepInstaller) installCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.inst
}

func (f *fakeDepInstaller) removeCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.remove
}

func newDeferredTestScheduler(installer DepInstaller, loader check.Loader) *CheckScheduler {
	s := &CheckScheduler{
		configToChecks:  make(map[string][]checkid.ID),
		deferredConfigs: make(map[string]deferredConfig),
		configDeps:      make(map[string][]string),
		depInstaller:    installer,
	}
	s.addLoader(loader)
	return s
}

func mysqlConfig() integration.Config {
	return integration.Config{
		Name:       "mysql",
		Instances:  []integration.Data{integration.Data(`{}`)},
		InitConfig: integration.Data(`{}`),
	}
}

func TestDeferAndUnDeferCheck(t *testing.T) {
	configmock.New(t)
	installer := &fakeDepInstaller{}
	loader := &pendingLoader{deps: []string{"datadog-agent-integration-deps"}}
	s := newDeferredTestScheduler(installer, loader)

	config := mysqlConfig()
	checks := s.GetChecksFromConfigs([]integration.Config{config}, true)
	assert.Empty(t, checks)
	require.Len(t, s.deferredConfigs, 1)
	for _, d := range s.deferredConfigs {
		assert.Equal(t, []string{"datadog-agent-integration-deps"}, d.deps)
	}
	configDigest := config.Digest()

	loader.setReady(true)
	s.retryDeferred()
	assert.Empty(t, s.deferredConfigs)
	assert.Contains(t, s.configDeps, configDigest)
	require.NotEmpty(t, s.configToChecks)
	require.Eventually(t, func() bool { return installer.installCalls() >= 1 },
		5*time.Second, 10*time.Millisecond, "an install request should have been triggered")
}

func TestDeferredCheckGivesUpAfterMaxAttempts(t *testing.T) {
	configmock.New(t)
	installer := &fakeDepInstaller{}
	loader := &pendingLoader{deps: []string{"datadog-agent-integration-deps"}}
	s := newDeferredTestScheduler(installer, loader)

	s.GetChecksFromConfigs([]integration.Config{mysqlConfig()}, true)
	for i := 0; i <= deferredMaxAttempts(); i++ {
		s.retryDeferred()
	}
	assert.Empty(t, s.deferredConfigs)
	assert.Contains(t, GetLoaderErrors(), "mysql")
}

func TestUnscheduleRemovesDeferredAndTriggersOrphanRemoval(t *testing.T) {
	prevGrace := depRemovalGrace
	depRemovalGrace = time.Millisecond
	defer func() { depRemovalGrace = prevGrace }()
	configmock.New(t)
	installer := &fakeDepInstaller{}
	loader := &pendingLoader{deps: []string{"datadog-agent-integration-deps"}}
	s := newDeferredTestScheduler(installer, loader)

	config := mysqlConfig()
	s.GetChecksFromConfigs([]integration.Config{config}, true)
	loader.setReady(true)
	s.retryDeferred()
	require.NotEmpty(t, s.configDeps)

	s.Unschedule([]integration.Config{config})
	assert.Empty(t, s.deferredConfigs)
	assert.Empty(t, s.configDeps)
	require.Eventually(t, func() bool { return installer.removeCalls() == 1 },
		5*time.Second, time.Millisecond, "orphan removal should have been requested")
}

func TestUnscheduleKeepsDepsWhileAnotherCheckUsesThem(t *testing.T) {
	prevGrace := depRemovalGrace
	depRemovalGrace = time.Millisecond
	defer func() { depRemovalGrace = prevGrace }()
	configmock.New(t)
	installer := &fakeDepInstaller{}
	loader := &pendingLoader{deps: []string{"datadog-agent-integration-deps"}}
	s := newDeferredTestScheduler(installer, loader)

	mysql := mysqlConfig()
	gunicorn := integration.Config{
		Name:       "gunicorn",
		Instances:  []integration.Data{integration.Data(`{}`)},
		InitConfig: integration.Data(`{}`),
	}
	s.GetChecksFromConfigs([]integration.Config{mysql}, true)
	loader.setReady(true)
	s.retryDeferred()
	s.GetChecksFromConfigs([]integration.Config{gunicorn}, true)

	s.Unschedule([]integration.Config{mysql})
	require.Never(t, func() bool { return installer.removeCalls() > 0 },
		time.Second, 10*time.Millisecond, "deps must not be removed while gunicorn still uses them")

	s.Unschedule([]integration.Config{gunicorn})
	require.Eventually(t, func() bool { return installer.removeCalls() == 1 },
		5*time.Second, time.Millisecond, "deps should be removed once no check uses them")
}
