// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package healthcheck

import (
	"context"
	"embed"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
)

func healthConfig() *integration.HealthCheckConfig {
	return &integration.HealthCheckConfig{
		Enabled:      true,
		ServiceCheck: "example.health",
		Remediation: integration.RemediationConfig{
			Cooldown:     "5m",
			MaxAttempts:  2,
			Steps:        []integration.RemediationStep{{Command: "/usr/bin/true"}},
			AllowedPaths: []string{"/usr/bin/true"},
		},
	}
}

func registerTestCheck(t *testing.T, cfg *integration.HealthCheckConfig) checkid.ID {
	t.Helper()
	id := checkid.ID(t.Name())
	Register(id, cfg)
	t.Cleanup(func() { Unregister(id) })
	return id
}

func TestRegistry(t *testing.T) {
	cfg := healthConfig()
	id := registerTestCheck(t, cfg)
	got, ok := Lookup(id)
	require.True(t, ok)
	require.Equal(t, cfg, got)
	cfg.Remediation.Steps[0].Command = "changed input"
	got.Remediation.AllowedPaths[0] = "changed output"
	got, ok = Lookup(id)
	require.True(t, ok)
	assert.Equal(t, "/usr/bin/true", got.Remediation.Steps[0].Command)
	assert.Equal(t, []string{"/usr/bin/true"}, got.Remediation.AllowedPaths)
	Unregister(id)
	got, ok = Lookup(id)
	assert.False(t, ok)
	assert.Nil(t, got)
	Unregister(id)
	Register(id, nil)
	Register(id, &integration.HealthCheckConfig{})
	Register("", healthConfig())
	_, ok = Lookup(id)
	assert.False(t, ok)
	_, ok = Lookup("")
	assert.False(t, ok)
}

func TestRegistryConcurrentAccess(t *testing.T) {
	id := registerTestCheck(t, healthConfig())
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 100 {
				Register(id, healthConfig())
				Lookup(id)
				Unregister(id)
			}
		})
	}
	wg.Wait()
}

type dispatchCall struct {
	id   checkid.ID
	name string
	cfg  *integration.HealthCheckConfig
}

type recordingDispatcher struct{ calls []dispatchCall }

func (d *recordingDispatcher) Dispatch(_ context.Context, id checkid.ID, name string, cfg *integration.HealthCheckConfig) {
	d.calls = append(d.calls, dispatchCall{id, name, cfg})
}

func TestObserverEdgesAndCooldownWindow(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cfg := healthConfig()
		id := registerTestCheck(t, cfg)
		d := &recordingDispatcher{}
		o := NewObserver(d)
		defer o.Stop()
		send := func(status servicecheck.ServiceCheckStatus) {
			o.ObserveServiceCheck(id, cfg.ServiceCheck, status)
			synctest.Wait()
		}
		send(servicecheck.ServiceCheckCritical)
		require.Empty(t, d.calls, "initial CRITICAL is not an OK-to-CRITICAL edge")
		send(servicecheck.ServiceCheckOK)
		send(servicecheck.ServiceCheckCritical)
		require.Len(t, d.calls, 1)
		assert.Equal(t, dispatchCall{id, cfg.ServiceCheck, cfg}, d.calls[0])
		send(servicecheck.ServiceCheckCritical)
		require.Len(t, d.calls, 1)
		send(servicecheck.ServiceCheckOK)
		send(servicecheck.ServiceCheckCritical)
		require.Len(t, d.calls, 2)
		send(servicecheck.ServiceCheckOK)
		send(servicecheck.ServiceCheckCritical)
		require.Len(t, d.calls, 2, "max attempts applies across repeated recoveries")
		time.Sleep(5 * time.Minute)
		send(servicecheck.ServiceCheckCritical)
		require.Len(t, d.calls, 2, "expiry alone does not create an edge")
		send(servicecheck.ServiceCheckOK)
		send(servicecheck.ServiceCheckCritical)
		require.Len(t, d.calls, 3)
	})
}

func TestObserverFiltersAndNonCriticalStatuses(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cfg := healthConfig()
		id := registerTestCheck(t, cfg)
		d := &recordingDispatcher{}
		o := NewObserver(d)
		defer o.Stop()
		for _, ignoredID := range []checkid.ID{"", "unregistered"} {
			o.ObserveServiceCheck(ignoredID, cfg.ServiceCheck, servicecheck.ServiceCheckOK)
			o.ObserveServiceCheck(ignoredID, cfg.ServiceCheck, servicecheck.ServiceCheckCritical)
		}
		o.ObserveServiceCheck(id, "other.health", servicecheck.ServiceCheckOK)
		o.ObserveServiceCheck(id, "other.health", servicecheck.ServiceCheckCritical)
		for _, status := range []servicecheck.ServiceCheckStatus{servicecheck.ServiceCheckOK, servicecheck.ServiceCheckWarning, servicecheck.ServiceCheckUnknown, servicecheck.ServiceCheckCritical} {
			o.ObserveServiceCheck(id, cfg.ServiceCheck, status)
			synctest.Wait()
		}
		require.Empty(t, d.calls)
		o.ObserveServiceCheck(id, cfg.ServiceCheck, servicecheck.ServiceCheckOK)
		synctest.Wait()
		o.ObserveServiceCheck(id, "other.health", servicecheck.ServiceCheckWarning)
		o.ObserveServiceCheck(id, cfg.ServiceCheck, servicecheck.ServiceCheckCritical)
		synctest.Wait()
		require.Len(t, d.calls, 1)
	})
}

func TestObserverWildcardUsesPerServiceEdgesAndSharedLimit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cfg := healthConfig()
		cfg.ServiceCheck = ""
		cfg.Remediation.MaxAttempts = 1
		id := registerTestCheck(t, cfg)
		d := &recordingDispatcher{}
		o := NewObserver(d)
		defer o.Stop()
		send := func(name string, status servicecheck.ServiceCheckStatus) {
			o.ObserveServiceCheck(id, name, status)
			synctest.Wait()
		}
		send("one", servicecheck.ServiceCheckOK)
		send("two", servicecheck.ServiceCheckCritical)
		require.Empty(t, d.calls, "different service checks must not create a false edge")
		send("one", servicecheck.ServiceCheckCritical)
		send("two", servicecheck.ServiceCheckOK)
		send("two", servicecheck.ServiceCheckCritical)
		require.Len(t, d.calls, 1)
		assert.Equal(t, "one", d.calls[0].name)
	})
}

func TestRegistryDefaultsAndStaleObservations(t *testing.T) {
	cfg := healthConfig()
	cfg.Remediation.Cooldown = "invalid"
	cfg.Remediation.MaxAttempts = -1
	id := registerTestCheck(t, cfg)
	registry.RLock()
	entry := registry.checks[id]
	registry.RUnlock()
	assert.Equal(t, defaultCooldown, entry.cooldown)
	assert.Equal(t, defaultMaxAttempts, entry.maxAttempts)
	obs := observation{id: id, generation: entry.generation, name: cfg.ServiceCheck, status: servicecheck.ServiceCheckOK, at: time.Now()}
	assert.Nil(t, transition(obs))
	Unregister(id)
	Register(id, cfg)
	obs.status = servicecheck.ServiceCheckCritical
	assert.Nil(t, transition(obs), "queued observations cannot apply to a new registration")
	registry.RLock()
	obs.generation = registry.checks[id].generation
	registry.RUnlock()
	assert.Nil(t, transition(obs), "unregister resets the prior OK")
	obs.status = servicecheck.ServiceCheckOK
	assert.Nil(t, transition(obs))
	obs.status = servicecheck.ServiceCheckCritical
	assert.NotNil(t, transition(obs))
}

type blockingDispatcher struct{ started chan struct{} }

func (d *blockingDispatcher) Dispatch(ctx context.Context, _ checkid.ID, _ string, _ *integration.HealthCheckConfig) {
	close(d.started)
	<-ctx.Done()
}

func TestObserverBackpressureAndShutdown(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cfg := healthConfig()
		id := registerTestCheck(t, cfg)
		d := &blockingDispatcher{started: make(chan struct{})}
		o := NewObserver(d)
		o.ObserveServiceCheck(id, cfg.ServiceCheck, servicecheck.ServiceCheckOK)
		synctest.Wait()
		o.ObserveServiceCheck(id, cfg.ServiceCheck, servicecheck.ServiceCheckCritical)
		<-d.started
		for range cap(o.input) + 1 {
			o.ObserveServiceCheck(id, cfg.ServiceCheck, servicecheck.ServiceCheckCritical)
		}
		assert.Equal(t, uint64(1), o.Dropped())
		o.Stop()
		o.Stop()
		before := len(o.input)
		o.ObserveServiceCheck(id, cfg.ServiceCheck, servicecheck.ServiceCheckOK)
		assert.Len(t, o.input, before)
	})
}

func TestObserverNil(t *testing.T) {
	var o *Observer
	o.ObserveServiceCheck("check", "health", servicecheck.ServiceCheckCritical)
	o.Stop()
	assert.Zero(t, o.Dropped())
	assert.Nil(t, NewObserver(nil))
}

func TestEventDispatcher(t *testing.T) {
	out := make(chan event.Event, 1)
	d := NewEventDispatcher(out, "agent-host")
	cfg := healthConfig()
	marker := filepath.Join(t.TempDir(), "must-not-exist")
	cfg.Remediation.Steps = []integration.RemediationStep{{Command: "touch " + marker}, {Command: "service example restart"}}
	d.Dispatch(context.Background(), "example:123", cfg.ServiceCheck, cfg)
	require.Len(t, out, 1)
	e := <-out
	assert.Equal(t, "health-check remediation (dry-run)", e.Title)
	assert.Contains(t, e.Text, "example:123")
	assert.Contains(t, e.Text, cfg.ServiceCheck)
	assert.Contains(t, e.Text, "1. touch "+marker)
	assert.Contains(t, e.Text, "2. service example restart")
	assert.Equal(t, "agent-host", e.Host)
	assert.Equal(t, event.AlertTypeInfo, e.AlertType)
	assert.Equal(t, event.PriorityNormal, e.Priority)
	assert.Equal(t, "datadog-agent", e.SourceTypeName)
	assert.Equal(t, "health_check_remediation:example:123", e.AggregationKey)
	assert.ElementsMatch(t, []string{"check_id:example:123", "service_check:" + cfg.ServiceCheck, "remediation:dry-run"}, e.Tags)
	assert.Positive(t, e.Ts)
	_, err := os.Stat(marker)
	assert.ErrorIs(t, err, os.ErrNotExist)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	d.Dispatch(ctx, "example:123", cfg.ServiceCheck, cfg)
	assert.Empty(t, out)
}

func TestEventDispatcherCancelsBlockedSend(t *testing.T) {
	synctest.Test(t, func(_ *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		d := NewEventDispatcher(make(chan event.Event), "host")
		go func() {
			d.Dispatch(ctx, "check", "health", healthConfig())
			close(done)
		}()
		synctest.Wait()
		cancel()
		<-done
	})
}

//go:embed *.go
var sources embed.FS

func TestNoHealthPlatformStoreImport(t *testing.T) {
	files, err := sources.ReadDir(".")
	require.NoError(t, err)
	require.NotEmpty(t, files)
	for _, entry := range files {
		path := entry.Name()
		source, err := sources.ReadFile(path)
		require.NoError(t, err)
		file, err := parser.ParseFile(token.NewFileSet(), path, source, parser.ImportsOnly)
		require.NoError(t, err)
		for _, spec := range file.Imports {
			name, err := strconv.Unquote(spec.Path.Value)
			require.NoError(t, err)
			assert.False(t, strings.Contains(name, "healthplatform/store"), "%s imports healthplatform/store", path)
		}
	}
}
