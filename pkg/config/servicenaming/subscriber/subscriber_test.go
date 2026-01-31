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
	wmeta.Push(workloadmeta.SourceRuntime, workloadmeta.Event{
		Type:   workloadmeta.EventTypeSet,
		Entity: testContainer,
	})

	// Wait for event processing using Eventually for reliability
	require.Eventually(t, func() bool {
		container, err := wmeta.GetContainer("test-container-123")
		if err != nil {
			return false
		}
		return container.CELServiceDiscovery != nil &&
			container.CELServiceDiscovery.ServiceName == "redis-service" &&
			container.CELServiceDiscovery.MatchedRule == "redis-rule"
	}, 2*time.Second, 10*time.Millisecond, "CELServiceDiscovery should be populated")

	// Final verification
	container, err := wmeta.GetContainer("test-container-123")
	require.NoError(t, err)
	require.NotNil(t, container.CELServiceDiscovery)
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
	wmeta.Push(workloadmeta.SourceRuntime, workloadmeta.Event{
		Type:   workloadmeta.EventTypeSet,
		Entity: testContainer,
	})

	// Use Never to assert that CELServiceDiscovery never gets set (negative assertion).
	// This repeatedly checks over a period to ensure the value stays nil.
	require.Never(t, func() bool {
		container, err := wmeta.GetContainer("unmatched-container")
		if err != nil {
			return false
		}
		// Return true if CELServiceDiscovery is NOT nil (which would fail the test)
		return container.CELServiceDiscovery != nil
	}, 200*time.Millisecond, 20*time.Millisecond, "CELServiceDiscovery should remain nil when no rule matches")
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

	wmeta.Push(workloadmeta.SourceRuntime, workloadmeta.Event{
		Type:   workloadmeta.EventTypeSet,
		Entity: container,
	})

	// Wait for event processing using Eventually for reliability
	require.Eventually(t, func() bool {
		result, err := wmeta.GetContainer("nil-metadata-container")
		if err != nil {
			return false
		}
		return result.CELServiceDiscovery != nil &&
			result.CELServiceDiscovery.ServiceName == "nginx"
	}, 2*time.Second, 10*time.Millisecond, "Should extract service name from image")

	// Final verification
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

	wmeta.Push(workloadmeta.SourceRuntime, workloadmeta.Event{
		Type:   workloadmeta.EventTypeSet,
		Entity: redisContainer,
	})
	wmeta.Push(workloadmeta.SourceRuntime, workloadmeta.Event{
		Type:   workloadmeta.EventTypeSet,
		Entity: postgresContainer,
	})
	wmeta.Push(workloadmeta.SourceRuntime, workloadmeta.Event{
		Type:   workloadmeta.EventTypeSet,
		Entity: nginxContainer,
	})

	// Wait for all three containers to be processed using Eventually
	require.Eventually(t, func() bool {
		redis, err1 := wmeta.GetContainer("redis-123")
		postgres, err2 := wmeta.GetContainer("postgres-456")
		nginx, err3 := wmeta.GetContainer("nginx-789")

		return err1 == nil && redis.CELServiceDiscovery != nil &&
			redis.CELServiceDiscovery.ServiceName == "redis-service" &&
			err2 == nil && postgres.CELServiceDiscovery != nil &&
			postgres.CELServiceDiscovery.ServiceName == "postgres-service" &&
			err3 == nil && nginx.CELServiceDiscovery != nil &&
			nginx.CELServiceDiscovery.ServiceName == "nginx"
	}, 2*time.Second, 10*time.Millisecond, "All containers should be processed")

	// Final verification
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

func TestSubscriber_ClearsStaleDataWhenRuleStopsMatching(t *testing.T) {
	// Test that when a container previously matched a rule but then changes so it no longer
	// matches, the CELServiceDiscovery is cleared to prevent stale data.
	yamlConfig := `
service_discovery:
  enabled: true
  service_definitions:
    - name: "redis-rule"
      query: "container['labels']['app'] == 'redis'"
      value: "'redis-service'"
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

	// Add a container that initially matches the redis rule
	testContainer := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   "changing-container",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:   "redis-container",
			Labels: map[string]string{"app": "redis"},
		},
		Image: workloadmeta.ContainerImage{
			ShortName: "redis",
		},
	}
	wmeta.Push(workloadmeta.SourceRuntime, workloadmeta.Event{
		Type:   workloadmeta.EventTypeSet,
		Entity: testContainer,
	})

	// Wait for service name to be set
	require.Eventually(t, func() bool {
		container, err := wmeta.GetContainer("changing-container")
		return err == nil && container.CELServiceDiscovery != nil &&
			container.CELServiceDiscovery.ServiceName == "redis-service"
	}, 2*time.Second, 10*time.Millisecond, "Service name should be set initially")

	// Now update the container so it no longer matches (change the label)
	updatedContainer := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   "changing-container",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:   "redis-container",
			Labels: map[string]string{"app": "not-redis"}, // Changed label
		},
		Image: workloadmeta.ContainerImage{
			ShortName: "redis",
		},
	}
	wmeta.Push(workloadmeta.SourceRuntime, workloadmeta.Event{
		Type:   workloadmeta.EventTypeSet,
		Entity: updatedContainer,
	})

	// Wait for CELServiceDiscovery to be cleared
	require.Eventually(t, func() bool {
		container, err := wmeta.GetContainer("changing-container")
		return err == nil && container.CELServiceDiscovery == nil
	}, 2*time.Second, 10*time.Millisecond, "Service name should be cleared when rule stops matching")

	// Final verification
	container, err := wmeta.GetContainer("changing-container")
	require.NoError(t, err)
	assert.Nil(t, container.CELServiceDiscovery, "CELServiceDiscovery should be nil after rule stops matching")
}

func TestSubscriber_CleansUpOnContainerDelete(t *testing.T) {
	// Test that when a container is deleted (EventTypeUnset), we clean up our
	// SourceServiceDiscovery contribution to prevent stale data from keeping the entity alive.
	yamlConfig := `
service_discovery:
  enabled: true
  service_definitions:
    - name: "test-rule"
      query: "true"
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

	// Add a container
	testContainer := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   "temporary-container",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: "temp",
		},
		Image: workloadmeta.ContainerImage{
			ShortName: "nginx",
		},
	}
	wmeta.Push(workloadmeta.SourceRuntime, workloadmeta.Event{
		Type:   workloadmeta.EventTypeSet,
		Entity: testContainer,
	})

	// Wait for service name to be set
	require.Eventually(t, func() bool {
		container, err := wmeta.GetContainer("temporary-container")
		return err == nil && container.CELServiceDiscovery != nil &&
			container.CELServiceDiscovery.ServiceName == "nginx"
	}, 2*time.Second, 10*time.Millisecond, "Service name should be set")

	// Now delete the container (EventTypeUnset from runtime)
	wmeta.Push(workloadmeta.SourceRuntime, workloadmeta.Event{
		Type:   workloadmeta.EventTypeUnset,
		Entity: testContainer,
	})

	// Give time for cleanup to process
	time.Sleep(100 * time.Millisecond)

	// Re-add the same container with a different image to verify cleanup happened
	// If cleanup didn't work, the subscriber might skip evaluation due to stale tracking
	readdedContainer := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   "temporary-container", // Same ID
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: "temp-v2",
		},
		Image: workloadmeta.ContainerImage{
			ShortName: "redis", // Different image
		},
	}
	wmeta.Push(workloadmeta.SourceRuntime, workloadmeta.Event{
		Type:   workloadmeta.EventTypeSet,
		Entity: readdedContainer,
	})

	// Verify the re-added container gets evaluated correctly with the new image name
	// This proves cleanup happened (tracking was cleared)
	require.Eventually(t, func() bool {
		container, err := wmeta.GetContainer("temporary-container")
		return err == nil && container.CELServiceDiscovery != nil &&
			container.CELServiceDiscovery.ServiceName == "redis"
	}, 2*time.Second, 10*time.Millisecond, "Re-added container should be evaluated with new properties")
}

func TestSubscriber_ReevaluatesWhenPortsChange(t *testing.T) {
	// Test that changes in ports trigger re-evaluation (hash includes ports)
	yamlConfig := `
service_discovery:
  enabled: true
  service_definitions:
    - name: "port-8080"
      query: "size(container['ports']) > 0 && container['ports'][0]['port'] == 8080"
      value: "'svc-8080'"
    - name: "fallback"
      query: "true"
      value: "'svc-other'"
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

	// Initial container with port 8080 -> should match port-8080 rule
	container := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   "port-change-container",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: "port-change",
		},
		Image: workloadmeta.ContainerImage{
			ShortName: "demo",
		},
		Ports: []workloadmeta.ContainerPort{
			{Name: "http", Port: 8080, Protocol: "tcp"},
		},
	}
	wmeta.Push(workloadmeta.SourceRuntime, workloadmeta.Event{
		Type:   workloadmeta.EventTypeSet,
		Entity: container,
	})

	require.Eventually(t, func() bool {
		c, err := wmeta.GetContainer("port-change-container")
		return err == nil && c.CELServiceDiscovery != nil &&
			c.CELServiceDiscovery.ServiceName == "svc-8080"
	}, 2*time.Second, 10*time.Millisecond, "Expected service name for port 8080")

	// Update ports to 9090 -> should fall back
	updated := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   "port-change-container",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: "port-change",
		},
		Image: workloadmeta.ContainerImage{
			ShortName: "demo",
		},
		Ports: []workloadmeta.ContainerPort{
			{Name: "http", Port: 9090, Protocol: "tcp"},
		},
	}
	wmeta.Push(workloadmeta.SourceRuntime, workloadmeta.Event{
		Type:   workloadmeta.EventTypeSet,
		Entity: updated,
	})

	require.Eventually(t, func() bool {
		c, err := wmeta.GetContainer("port-change-container")
		return err == nil && c.CELServiceDiscovery != nil &&
			c.CELServiceDiscovery.ServiceName == "svc-other"
	}, 2*time.Second, 10*time.Millisecond, "Expected fallback service name after port change")
}
