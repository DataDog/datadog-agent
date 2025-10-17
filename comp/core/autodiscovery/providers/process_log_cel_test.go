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
	t.Run("with CEL filter - excludes nginx process", func(t *testing.T) {
		// Create a mock config with CEL rules to exclude nginx
		mockConfig := configmock.NewFromYAML(t, `
cel_workload_exclude:
- products: ["logs"]
  rules:
    processes:
      - "process.name == 'nginx'"
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

		// Verify filter was initialized
		assert.NotNil(t, p.logsFilters, "Filter should be initialized when filter component provided")

		nginxLogPath := "/var/log/nginx.log"
		javaLogPath := "/var/log/java.log"

		processes := []workloadmeta.Event{
			{
				Type: workloadmeta.EventTypeSet,
				Entity: &workloadmeta.Process{
					EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindProcess, ID: "123"},
					Name:     "nginx",
					Pid:      123,
					Cmdline:  []string{"nginx", "-g", "daemon off;"},
					Service: &workloadmeta.Service{
						GeneratedName: "nginx",
						LogFiles:      []string{nginxLogPath},
					},
				},
			},
			{
				Type: workloadmeta.EventTypeSet,
				Entity: &workloadmeta.Process{
					EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindProcess, ID: "456"},
					Name:     "java",
					Pid:      456,
					Cmdline:  []string{"java", "-jar", "app.jar"},
					Service: &workloadmeta.Service{
						GeneratedName: "java",
						LogFiles:      []string{javaLogPath},
					},
				},
			},
		}

		setBundle := workloadmeta.EventBundle{Events: processes}
		changes := p.processEventsNoVerifyReadable(setBundle)

		// With CEL filter, only java process should be scheduled (nginx is excluded)
		assert.Len(t, changes.Schedule, 1, "Only java process should be scheduled (nginx excluded by CEL filter)")
		assert.Len(t, changes.Unschedule, 0)

		// Verify only java config was created
		config := changes.Schedule[0]
		assert.Equal(t, getIntegrationName(javaLogPath), config.Name)
		assert.Contains(t, string(config.LogsConfig), javaLogPath)
		assert.NotContains(t, string(config.LogsConfig), nginxLogPath)
	})
}
