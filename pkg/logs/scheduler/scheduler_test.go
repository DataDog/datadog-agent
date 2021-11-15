// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package scheduler

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/service"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	workloadmetatesting "github.com/DataDog/datadog-agent/pkg/workloadmeta/testing"
	"github.com/stretchr/testify/assert"
)

var Container1 = "a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b"
var Container2 = "9bad87700e8dac88d607056119ce80d3f33da461308d3d60367018891be5ee37"

func TestScheduleConfigCreatesNewSource(t *testing.T) {
	logSources := config.NewLogSources()
	services := service.NewServices()
	s := NewScheduler(logSources, services, workloadmetatesting.NewStore())
	defer s.Stop()

	logSourcesStream := logSources.GetAddedForType(config.DockerType)

	configSource := integration.Config{
		LogsConfig:    []byte(`[{"service":"foo","source":"bar"}]`),
		ADIdentifiers: []string{"docker://" + Container1},
		Provider:      names.Kubernetes,
		TaggerEntity:  "container_id://" + Container1,
		Entity:        "docker://" + Container1,
		ClusterCheck:  false,
		CreationTime:  0,
	}

	go s.Schedule([]integration.Config{configSource})
	logSource := <-logSourcesStream
	assert.Equal(t, config.DockerType, logSource.Name)
	// We use the docker socket, not sourceType here
	assert.Equal(t, config.SourceType(""), logSource.GetSourceType())
	assert.Equal(t, "foo", logSource.Config.Service)
	assert.Equal(t, "bar", logSource.Config.Source)
	assert.Equal(t, config.DockerType, logSource.Config.Type)
	assert.Equal(t, Container1, logSource.Config.Identifier)
}

func TestScheduleConfigCreatesNewSourceServiceFallback(t *testing.T) {
	logSources := config.NewLogSources()
	services := service.NewServices()
	s := NewScheduler(logSources, services, workloadmetatesting.NewStore())
	defer s.Stop()

	logSourcesStream := logSources.GetAddedForType(config.DockerType)

	configSource := integration.Config{
		InitConfig:    []byte(`{"service":"foo"}`),
		LogsConfig:    []byte(`[{"source":"bar"}]`),
		ADIdentifiers: []string{"docker://" + Container1},
		Provider:      names.Kubernetes,
		TaggerEntity:  "container_id://" + Container1,
		Entity:        "docker://" + Container1,
		ClusterCheck:  false,
		CreationTime:  0,
	}

	go s.Schedule([]integration.Config{configSource})
	logSource := <-logSourcesStream
	assert.Equal(t, config.DockerType, logSource.Name)
	// We use the docker socket, not sourceType here
	assert.Equal(t, config.SourceType(""), logSource.GetSourceType())
	assert.Equal(t, "foo", logSource.Config.Service)
	assert.Equal(t, "bar", logSource.Config.Source)
	assert.Equal(t, config.DockerType, logSource.Config.Type)
	assert.Equal(t, Container1, logSource.Config.Identifier)
}

func TestScheduleConfigCreatesNewSourceServiceOverride(t *testing.T) {
	logSources := config.NewLogSources()
	services := service.NewServices()
	s := NewScheduler(logSources, services, workloadmetatesting.NewStore())
	defer s.Stop()

	logSourcesStream := logSources.GetAddedForType(config.DockerType)

	configSource := integration.Config{
		InitConfig:    []byte(`{"service":"foo"}`),
		LogsConfig:    []byte(`[{"source":"bar","service":"baz"}]`),
		ADIdentifiers: []string{"docker://" + Container1},
		Provider:      names.Kubernetes,
		TaggerEntity:  "container_id://" + Container1,
		Entity:        "docker://" + Container1,
		ClusterCheck:  false,
		CreationTime:  0,
	}

	go s.Schedule([]integration.Config{configSource})
	logSource := <-logSourcesStream
	assert.Equal(t, config.DockerType, logSource.Name)
	// We use the docker socket, not sourceType here
	assert.Equal(t, config.SourceType(""), logSource.GetSourceType())
	assert.Equal(t, "baz", logSource.Config.Service)
	assert.Equal(t, "bar", logSource.Config.Source)
	assert.Equal(t, config.DockerType, logSource.Config.Type)
	assert.Equal(t, Container1, logSource.Config.Identifier)
}

func TestSetEventCreatesNewContainerService(t *testing.T) {
	logSources := config.NewLogSources()
	services := service.NewServices()
	store := workloadmetatesting.NewStore()
	s := NewScheduler(logSources, services, store)
	defer s.Stop()

	servicesStream := services.GetAddedServicesForType(config.DockerType)

	makeEvent := func(id string) workloadmeta.Event {
		return workloadmeta.Event{
			Type: workloadmeta.EventTypeSet,
			Entity: &workloadmeta.Container{
				Runtime:  workloadmeta.ContainerRuntimeDocker,
				EntityID: workloadmeta.EntityID{ID: id},
			},
		}
	}

	go store.NotifySubscribers([]workloadmeta.Event{
		makeEvent(Container1),
	})

	svc := <-servicesStream
	assert.Equal(t, "docker://"+Container1, svc.GetEntityID())
	assert.Equal(t, service.Before, svc.CreationTime)

	// next notification (for a new container) should be "After"
	go store.NotifySubscribers([]workloadmeta.Event{
		makeEvent(Container2),
	})

	svc = <-servicesStream
	assert.Equal(t, "docker://"+Container2, svc.GetEntityID())
	assert.Equal(t, service.After, svc.CreationTime)

	// notification for same container as the first should not result in a new service
	go store.NotifySubscribers([]workloadmeta.Event{
		makeEvent(Container1),
	})

	select {
	case svc = <-servicesStream:
		assert.Fail(t, "should not have produced a service", "got service %#v", svc)
	case <-time.After(10 * time.Millisecond):
		// all good!
	}
}

func TestUnscheduleConfigRemovesSource(t *testing.T) {
	logSources := config.NewLogSources()
	services := service.NewServices()
	s := NewScheduler(logSources, services, workloadmetatesting.NewStore())
	defer s.Stop()
	logSourcesStream := logSources.GetRemovedForType(config.DockerType)

	configSource := integration.Config{
		LogsConfig:    []byte(`[{"service":"foo","source":"bar"}]`),
		ADIdentifiers: []string{"docker://" + Container1},
		Provider:      names.Kubernetes,
		TaggerEntity:  "container_id://" + Container1,
		Entity:        "docker://" + Container1,
		ClusterCheck:  false,
		CreationTime:  0,
	}

	// We need to have a source to remove
	sources, _ := s.toSources(configSource)
	logSources.AddSource(sources[0])

	go s.Unschedule([]integration.Config{configSource})
	logSource := <-logSourcesStream
	assert.Equal(t, config.DockerType, logSource.Name)
	// We use the docker socket, not sourceType here
	assert.Equal(t, config.SourceType(""), logSource.GetSourceType())
	assert.Equal(t, "foo", logSource.Config.Service)
	assert.Equal(t, "bar", logSource.Config.Source)
	assert.Equal(t, config.DockerType, logSource.Config.Type)
	assert.Equal(t, Container1, logSource.Config.Identifier)
}

func TestUnsetEventRemovesService(t *testing.T) {
	logSources := config.NewLogSources()
	services := service.NewServices()
	store := workloadmetatesting.NewStore()
	s := NewScheduler(logSources, services, store)
	defer s.Stop()

	addedServicesStream := services.GetAddedServicesForType(config.DockerType)
	removedServicesStream := services.GetRemovedServicesForType(config.DockerType)

	// first set the service
	go store.NotifySubscribers([]workloadmeta.Event{
		{
			Type: workloadmeta.EventTypeSet,
			Entity: &workloadmeta.Container{
				Runtime: workloadmeta.ContainerRuntimeDocker,
				EntityID: workloadmeta.EntityID{
					ID: Container1,
				},
			},
		},
	})

	svc := <-addedServicesStream
	assert.Equal(t, "docker://"+Container1, svc.GetEntityID())

	// now unset it
	go store.NotifySubscribers([]workloadmeta.Event{
		{
			Type: workloadmeta.EventTypeUnset,
			Entity: &workloadmeta.EntityID{
				ID: Container1,
			},
		},
	})

	svc = <-removedServicesStream
	assert.Equal(t, "docker://"+Container1, svc.GetEntityID())
}

func TestIgnoreConfigIfLogsExcluded(t *testing.T) {
	logSources := config.NewLogSources()
	services := service.NewServices()
	s := NewScheduler(logSources, services, workloadmetatesting.NewStore())
	defer s.Stop()
	servicesStreamIn := services.GetAddedServicesForType(config.DockerType)
	servicesStreamOut := services.GetRemovedServicesForType(config.DockerType)

	configService := integration.Config{
		LogsConfig:   []byte(""),
		TaggerEntity: "container_id://" + Container1,
		Entity:       "docker://" + Container1,
		ClusterCheck: false,
		CreationTime: 0,
		LogsExcluded: true,
	}

	go s.Schedule([]integration.Config{configService})
	select {
	case <-servicesStreamIn:
		assert.Fail(t, "config must be ignored")
	case <-time.After(100 * time.Millisecond):
		break
	}

	go s.Unschedule([]integration.Config{configService})
	select {
	case <-servicesStreamOut:
		assert.Fail(t, "config must be ignored")
	case <-time.After(100 * time.Millisecond):
		break
	}
}
