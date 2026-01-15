// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build cel

package providers

import (
	"testing"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	noopTelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/noopsimpl"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadfilterimpl "github.com/DataDog/datadog-agent/comp/core/workloadfilter/impl"
	workloadfiltermock "github.com/DataDog/datadog-agent/comp/core/workloadfilter/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessLogProviderCELFilter(t *testing.T) {
	// Create a mock config with CEL rules covering multiple scenarios
	mockConfig := configmock.NewFromYAML(t, `
cel_workload_exclude:
- products: ["global"]
  rules:
    processes:
      - "process.log_file.endsWith('.debug.log')"
- products: ["logs"]
  rules:
    processes:
      - "process.name == 'nginx'"
      - "!process.log_file.startsWith('/var/log/app/')"
`)

	// Create a real workloadfilter with the mock config
	filter := fxutil.Test[workloadfiltermock.Mock](t, fx.Options(
		fx.Provide(func() config.Component { return mockConfig }),
		fx.Provide(func() log.Component { return logmock.New(t) }),
		noopTelemetry.Module(),
		fxutil.Component(
			fxutil.ProvideComponentConstructor(workloadfilterimpl.NewMock),
			fx.Provide(func(mock workloadfiltermock.Mock) workloadfilter.Component { return mock }),
		),
	))

	// Create provider with the filter
	provider, err := NewProcessLogConfigProvider(nil, nil, nil, filter, nil)
	require.NoError(t, err)

	p, ok := provider.(*processLogConfigProvider)
	require.True(t, ok)
	assert.NotNil(t, p.logsFilters, "Filter should be initialized when filter component provided")

	// Test 1: Process excluded by name
	nginxProcess := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindProcess, ID: "123"},
		Name:     "nginx",
		Pid:      123,
		Cmdline:  []string{"nginx", "-g", "daemon off;"},
		Service: &workloadmeta.Service{
			GeneratedName: "nginx",
			LogFiles:      []string{"/var/log/nginx.log"},
		},
	}

	// Test 2: Process with multiple log files - some excluded by extension
	myappProcess := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindProcess, ID: "456"},
		Name:     "myapp",
		Pid:      456,
		Cmdline:  []string{"myapp", "--config", "/etc/myapp.conf"},
		Service: &workloadmeta.Service{
			GeneratedName: "myapp",
			LogFiles:      []string{"/var/log/app/service.log", "/var/log/app/app.debug.log", "/var/log/system.log"},
		},
	}

	// Test 3: Process that should be included (not nginx, has allowed log file)
	javaProcess := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindProcess, ID: "789"},
		Name:     "java",
		Pid:      789,
		Cmdline:  []string{"java", "-jar", "app.jar"},
		Service: &workloadmeta.Service{
			GeneratedName: "java",
			LogFiles:      []string{"/var/log/app/java.log"},
		},
	}

	setBundle := workloadmeta.EventBundle{
		Events: []workloadmeta.Event{
			{Type: workloadmeta.EventTypeSet, Entity: nginxProcess},
			{Type: workloadmeta.EventTypeSet, Entity: myappProcess},
			{Type: workloadmeta.EventTypeSet, Entity: javaProcess},
		},
	}

	changes := p.processEventsNoVerifyReadable(setBundle)

	// Only 2 configs should be scheduled:
	// - /var/log/app/service.log from myapp (excluded: app.debug.log by extension, system.log by path)
	// - /var/log/app/java.log from java
	// nginx is completely excluded by name
	assert.Len(t, changes.Schedule, 2, "Expected 2 scheduled configs")
	assert.Len(t, changes.Unschedule, 0)

	scheduleMap := scheduleToMap(changes.Schedule)

	// Verify service.log from myapp is scheduled
	serviceConfig, foundService := scheduleMap[getIntegrationName("/var/log/app/service.log")]
	assert.True(t, foundService, "/var/log/app/service.log should be scheduled")
	assert.Contains(t, string(serviceConfig.LogsConfig), "/var/log/app/service.log")

	// Verify java.log is scheduled
	javaConfig, foundJava := scheduleMap[getIntegrationName("/var/log/app/java.log")]
	assert.True(t, foundJava, "/var/log/app/java.log should be scheduled")
	assert.Contains(t, string(javaConfig.LogsConfig), "/var/log/app/java.log")

	// Verify excluded logs are NOT scheduled
	_, foundNginx := scheduleMap[getIntegrationName("/var/log/nginx.log")]
	assert.False(t, foundNginx, "nginx.log should NOT be scheduled (excluded by process name)")

	_, foundDebug := scheduleMap[getIntegrationName("/var/log/app/app.debug.log")]
	assert.False(t, foundDebug, "app.debug.log should NOT be scheduled (excluded by extension)")

	_, foundSystem := scheduleMap[getIntegrationName("/var/log/system.log")]
	assert.False(t, foundSystem, "system.log should NOT be scheduled (excluded by path pattern)")
}
