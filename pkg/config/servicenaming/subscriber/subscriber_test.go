// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build cel && test

package subscriber

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestNewSubscriber_Disabled(t *testing.T) {
	// When service_discovery.enabled is false, NewSubscriber should return nil
	wmeta := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	cfg := wmeta.GetConfig()

	sub, err := NewSubscriber(cfg, wmeta)
	require.NoError(t, err)
	assert.Nil(t, sub, "subscriber should be nil when disabled")
}

func TestNewSubscriber_InvalidRule(t *testing.T) {
	// When a rule has invalid CEL syntax, NewSubscriber should return an error
	yamlConfig := `
service_discovery:
  enabled: true
  service_definitions:
    - query: "invalid syntax {{{"
      value: "'test'"
`
	wmeta := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		fx.Provide(func() log.Component { return logmock.New(t) }),
		fx.Provide(func() config.Component {
			return config.NewMockFromYAML(t, yamlConfig)
		}),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	cfg := wmeta.GetConfig()

	_, err := NewSubscriber(cfg, wmeta)
	require.Error(t, err, "should return error for invalid CEL syntax")
}

func TestSubscriber_WritesServiceNameOnMatch(t *testing.T) {
	// Test that when a container matches a rule, the service name is written to workloadmeta
	yamlConfig := `
service_discovery:
  enabled: true
  service_definitions:
    - name: "redis-rule"
      query: "container['labels']['app'] == 'redis'"
      value: "'redis-service'"
    - query: "true"
      value: "container['image']['shortname']"
`
	wmeta := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		fx.Provide(func() log.Component { return logmock.New(t) }),
		fx.Provide(func() config.Component {
			return config.NewMockFromYAML(t, yamlConfig)
		}),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	cfg := wmeta.GetConfig()

	sub, err := NewSubscriber(cfg, wmeta)
	require.NoError(t, err)
	require.NotNil(t, sub, "subscriber should be created")

	// Start subscriber in background
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go sub.Start(ctx)

	// Give subscriber time to subscribe
	time.Sleep(50 * time.Millisecond)

	// Add a container that matches the redis rule
	testContainer := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   "test-container-123",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:   "redis-container",
			Labels: map[string]string{"app": "redis"},
		},
		Image: workloadmeta.ContainerImage{
			Name:      "redis:latest",
			ShortName: "redis",
			Tag:       "latest",
		},
	}
	wmeta.Set(testContainer)

	// Wait for event processing
	time.Sleep(100 * time.Millisecond)

	// Verify the container now has CELServiceDiscovery
	container, err := wmeta.GetContainer("test-container-123")
	require.NoError(t, err)
	require.NotNil(t, container.CELServiceDiscovery, "CELServiceDiscovery should be populated")
	assert.Equal(t, "redis-service", container.CELServiceDiscovery.ServiceName)
	assert.Equal(t, "redis-rule", container.CELServiceDiscovery.MatchedRule)
}

func TestSubscriber_NoMatchDoesNotWriteMetadata(t *testing.T) {
	// When no rule matches, no CELServiceDiscovery should be written
	yamlConfig := `
service_discovery:
  enabled: true
  service_definitions:
    - query: "container['labels']['special'] == 'yes'"
      value: "'special-service'"
`
	wmeta := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		fx.Provide(func() log.Component { return logmock.New(t) }),
		fx.Provide(func() config.Component {
			return config.NewMockFromYAML(t, yamlConfig)
		}),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	cfg := wmeta.GetConfig()

	sub, err := NewSubscriber(cfg, wmeta)
	require.NoError(t, err)
	require.NotNil(t, sub)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go sub.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	// Add a container that doesn't match any rule
	testContainer := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   "unmatched-container",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:   "unmatched",
			Labels: map[string]string{"other": "label"},
		},
		Image: workloadmeta.ContainerImage{
			ShortName: "generic-app",
		},
	}
	wmeta.Set(testContainer)

	time.Sleep(100 * time.Millisecond)

	container, err := wmeta.GetContainer("unmatched-container")
	require.NoError(t, err)
	// CELServiceDiscovery should be nil since no rule matched
	assert.Nil(t, container.CELServiceDiscovery, "CELServiceDiscovery should be nil when no rule matches")
}

func TestBuildCELInput(t *testing.T) {
	container := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   "container-id-123",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:   "my-container",
			Labels: map[string]string{"app": "myapp", "env": "prod"},
		},
		EnvVars: map[string]string{"DD_SERVICE": "my-service"},
		Image: workloadmeta.ContainerImage{
			Name:      "docker.io/myorg/myimage:v1.0",
			ShortName: "myimage",
			Tag:       "v1.0",
			Registry:  "docker.io",
		},
		Ports: []workloadmeta.ContainerPort{
			{Name: "http", Port: 8080, Protocol: "tcp"},
			{Name: "metrics", Port: 9090, Protocol: "tcp"},
		},
	}

	input := buildCELInput(container)

	require.NotNil(t, input.Container)
	assert.Equal(t, "container-id-123", input.Container.ID)
	assert.Equal(t, "my-container", input.Container.Name)
	assert.Equal(t, "myimage", input.Container.Image.ShortName)
	assert.Equal(t, "v1.0", input.Container.Image.Tag)
	assert.Equal(t, "docker.io", input.Container.Image.Registry)
	assert.Equal(t, "myapp", input.Container.Labels["app"])
	assert.Equal(t, "my-service", input.Container.Envs["DD_SERVICE"])
	assert.Len(t, input.Container.Ports, 2)
	assert.Equal(t, 8080, input.Container.Ports[0].Port)
	assert.Equal(t, "http", input.Container.Ports[0].Name)
}

func TestProcessContainer_NilLabelsAndEnvs(t *testing.T) {
	// Test that nil labels and envs are handled gracefully
	yamlConfig := `
service_discovery:
  enabled: true
  service_definitions:
    - query: "true"
      value: "container['image']['shortname']"
`
	wmeta := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		fx.Provide(func() log.Component { return logmock.New(t) }),
		fx.Provide(func() config.Component {
			return config.NewMockFromYAML(t, yamlConfig)
		}),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	cfg := wmeta.GetConfig()
	sub, err := NewSubscriber(cfg, wmeta)
	require.NoError(t, err)
	require.NotNil(t, sub)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go sub.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	// Container with nil labels and envs
	container := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   "nil-metadata-container",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:   "test",
			Labels: nil, // nil labels
		},
		EnvVars: nil, // nil envs
		Image: workloadmeta.ContainerImage{
			ShortName: "nginx",
		},
	}

	wmeta.Set(container)
	time.Sleep(100 * time.Millisecond)

	// Should still work and extract service name from image
	result, err := wmeta.GetContainer("nil-metadata-container")
	require.NoError(t, err)
	require.NotNil(t, result.CELServiceDiscovery)
	assert.Equal(t, "nginx", result.CELServiceDiscovery.ServiceName)
}

func TestSubscriber_MultipleContainersInBundle(t *testing.T) {
	// Test that multiple containers in a single event bundle are all processed
	yamlConfig := `
service_discovery:
  enabled: true
  service_definitions:
    - query: "container['labels']['app'] == 'redis'"
      value: "'redis-service'"
    - query: "container['labels']['app'] == 'postgres'"
      value: "'postgres-service'"
    - query: "true"
      value: "container['image']['shortname']"
`
	wmeta := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		fx.Provide(func() log.Component { return logmock.New(t) }),
		fx.Provide(func() config.Component {
			return config.NewMockFromYAML(t, yamlConfig)
		}),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	cfg := wmeta.GetConfig()
	sub, err := NewSubscriber(cfg, wmeta)
	require.NoError(t, err)
	require.NotNil(t, sub)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go sub.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	// Add multiple containers
	redisContainer := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   "redis-123",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:   "redis",
			Labels: map[string]string{"app": "redis"},
		},
		Image: workloadmeta.ContainerImage{ShortName: "redis"},
	}

	postgresContainer := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   "postgres-456",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:   "postgres",
			Labels: map[string]string{"app": "postgres"},
		},
		Image: workloadmeta.ContainerImage{ShortName: "postgres"},
	}

	nginxContainer := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   "nginx-789",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:   "nginx",
			Labels: map[string]string{"tier": "frontend"},
		},
		Image: workloadmeta.ContainerImage{ShortName: "nginx"},
	}

	wmeta.Set(redisContainer)
	wmeta.Set(postgresContainer)
	wmeta.Set(nginxContainer)

	time.Sleep(150 * time.Millisecond)

	// Verify all three containers were processed correctly
	redis, err := wmeta.GetContainer("redis-123")
	require.NoError(t, err)
	require.NotNil(t, redis.CELServiceDiscovery)
	assert.Equal(t, "redis-service", redis.CELServiceDiscovery.ServiceName)

	postgres, err := wmeta.GetContainer("postgres-456")
	require.NoError(t, err)
	require.NotNil(t, postgres.CELServiceDiscovery)
	assert.Equal(t, "postgres-service", postgres.CELServiceDiscovery.ServiceName)

	nginx, err := wmeta.GetContainer("nginx-789")
	require.NoError(t, err)
	require.NotNil(t, nginx.CELServiceDiscovery)
	assert.Equal(t, "nginx", nginx.CELServiceDiscovery.ServiceName)
}
