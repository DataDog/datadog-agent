// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collector

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	collectorcomp "github.com/DataDog/datadog-agent/comp/collector/collector/def"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/healthcheck"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

func TestLoadCheckInstanceHealthCheckRegistration(t *testing.T) {
	for _, tt := range []struct {
		name           string
		featureEnabled bool
		healthCheck    *integration.HealthCheckConfig
		registered     bool
	}{
		{name: "feature disabled", healthCheck: &integration.HealthCheckConfig{Enabled: true}},
		{name: "no declaration", featureEnabled: true},
		{name: "declaration disabled", featureEnabled: true, healthCheck: &integration.HealthCheckConfig{}},
		{name: "enabled", featureEnabled: true, healthCheck: &integration.HealthCheckConfig{Enabled: true, ServiceCheck: "http.can_connect"}, registered: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := configmock.New(t)
			cfg.SetInTest("health_check_remediation.enabled", tt.featureEnabled)
			id := checkid.ID(t.Name())
			t.Cleanup(func() { healthcheck.Unregister(id) })
			s := CheckScheduler{}
			s.addLoader(&recordingSchedulerLoader{name: "core", normalCheckID: id})
			config := integration.Config{Name: "http_check", HealthCheck: tt.healthCheck}
			result := s.loadCheckInstance(&recordingSchedulerSenderManager{}, config, integration.Data("{}"), 0, "")
			require.NotNil(t, result.check)
			registered, found := healthcheck.Lookup(id)
			assert.Equal(t, tt.registered, found)
			if tt.registered {
				assert.Equal(t, tt.healthCheck, registered)
			}
		})
	}
}

func TestScheduleHealthCheckLifecycle(t *testing.T) {
	for _, fail := range []bool{false, true} {
		name := "unschedule removes registration"
		if fail {
			name = "failed schedule removes registration"
		}
		t.Run(name, func(t *testing.T) {
			cfg := configmock.New(t)
			cfg.SetInTest("health_check_remediation.enabled", true)
			id := checkid.ID(t.Name())
			t.Cleanup(func() { healthcheck.Unregister(id) })
			coll := &MockCollector{}
			if fail {
				coll.RunCheckError = errors.New("cannot schedule check")
			}
			s := CheckScheduler{
				collector:      option.New[collectorcomp.Component](coll),
				configToChecks: make(map[string][]checkid.ID),
				senderManager:  &recordingSchedulerSenderManager{},
			}
			s.addLoader(&recordingSchedulerLoader{name: "core", normalCheckID: id})
			config := integration.Config{
				Name:        "http_check",
				InitConfig:  integration.Data("{}"),
				Instances:   []integration.Data{integration.Data("{}")},
				HealthCheck: &integration.HealthCheckConfig{Enabled: true},
			}
			s.Schedule([]integration.Config{config})
			require.Len(t, coll.RunCheckCalls, 1)
			_, found := healthcheck.Lookup(id)
			assert.Equal(t, !fail, found)
			s.Unschedule([]integration.Config{config})
			_, found = healthcheck.Lookup(id)
			assert.False(t, found)
			assert.NotContains(t, s.configToChecks, config.Digest())
		})
	}
}

type healthCheckLifecycleCollector struct {
	MockCollector
	running           []check.Check
	stopError         error
	retainOnStopError bool
}

func (c *healthCheckLifecycleCollector) RunCheck(ch check.Check) (checkid.ID, error) {
	c.RunCheckCalls = append(c.RunCheckCalls, ch)
	for _, running := range c.running {
		if running.ID() == ch.ID() {
			return "", errors.New("check already running")
		}
	}
	c.running = append(c.running, ch)
	return ch.ID(), nil
}

func (c *healthCheckLifecycleCollector) GetChecks() []check.Check {
	return c.running
}

func (c *healthCheckLifecycleCollector) StopCheck(id checkid.ID) error {
	if !c.retainOnStopError {
		for i, ch := range c.running {
			if ch.ID() == id {
				c.running = append(c.running[:i], c.running[i+1:]...)
				break
			}
		}
	}
	return c.stopError
}

func newHealthCheckLifecycleScheduler(t *testing.T) (*CheckScheduler, *healthCheckLifecycleCollector, integration.Config, checkid.ID) {
	t.Helper()
	cfg := configmock.New(t)
	cfg.SetInTest("health_check_remediation.enabled", true)
	id := checkid.ID(t.Name())
	t.Cleanup(func() { healthcheck.Unregister(id) })
	coll := &healthCheckLifecycleCollector{}
	s := &CheckScheduler{
		collector:      option.New[collectorcomp.Component](coll),
		configToChecks: make(map[string][]checkid.ID),
		senderManager:  &recordingSchedulerSenderManager{},
	}
	s.addLoader(&recordingSchedulerLoader{name: "core", normalCheckID: id})
	config := integration.Config{
		Name:        "http_check",
		InitConfig:  integration.Data("{}"),
		Instances:   []integration.Data{integration.Data("{}")},
		HealthCheck: &integration.HealthCheckConfig{Enabled: true, ServiceCheck: "http.can_connect"},
	}
	return s, coll, config, id
}

func TestUnscheduleHealthCheckStopError(t *testing.T) {
	for _, retain := range []bool{false, true} {
		name := "stop timeout removes registration"
		if retain {
			name = "schedule cancellation failure preserves registration"
		}
		t.Run(name, func(t *testing.T) {
			s, coll, config, id := newHealthCheckLifecycleScheduler(t)
			s.Schedule([]integration.Config{config})
			_, found := healthcheck.Lookup(id)
			require.True(t, found)
			coll.stopError = errors.New("stop failed")
			coll.retainOnStopError = retain
			s.Unschedule([]integration.Config{config})
			_, found = healthcheck.Lookup(id)
			assert.Equal(t, retain, found)
			_, cached := s.configToChecks[config.Digest()]
			assert.Equal(t, retain, cached)
		})
	}
}

func TestScheduleHealthCheckDuplicates(t *testing.T) {
	for _, reschedule := range []bool{false, true} {
		name := "identical instances"
		if reschedule {
			name = "rescheduled config"
		}
		t.Run(name, func(t *testing.T) {
			s, coll, config, id := newHealthCheckLifecycleScheduler(t)
			if !reschedule {
				config.Instances = append(config.Instances, config.Instances[0])
			}
			s.Schedule([]integration.Config{config})
			original, found := healthcheck.Lookup(id)
			require.True(t, found)
			if reschedule {
				// A duplicate load must not overwrite the running check's declaration.
				config.HealthCheck.ServiceCheck = "http.response_time"
				s.Schedule([]integration.Config{config})
			}
			require.Len(t, coll.RunCheckCalls, 2)
			require.Len(t, coll.running, 1)
			assert.Same(t, coll.RunCheckCalls[0], coll.running[0])
			registered, found := healthcheck.Lookup(id)
			require.True(t, found)
			assert.Equal(t, original, registered)
		})
	}
}
