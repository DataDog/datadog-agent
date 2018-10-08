// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package scheduler

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/providers"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/service"
	"github.com/stretchr/testify/assert"
)

func TestScheduleConfigCreatesNewSource(t *testing.T) {
	logSources := config.NewLogSources()
	services := service.NewServices()
	scheduler := NewScheduler(logSources, services)

	logSourcesStream := logSources.GetAddedForType(config.DockerType)

	configSource := integration.Config{
		LogsConfig:    []byte(`[{"service":"foo","source":"bar"}]`),
		ADIdentifiers: []string{"docker://a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b"},
		Provider:      providers.Kubernetes,
		Entity:        "docker://a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b",
		ClusterCheck:  false,
		CreationTime:  0,
	}

	go scheduler.Schedule([]integration.Config{configSource})
	logSource := <-logSourcesStream
	assert.Equal(t, config.DockerType, logSource.Name)
	// We use the docker socket, not sourceType here
	assert.Equal(t, "", logSource.GetSourceType())
	assert.Equal(t, "foo", logSource.Config.Service)
	assert.Equal(t, "bar", logSource.Config.Source)
	assert.Equal(t, config.DockerType, logSource.Config.Type)
	assert.Equal(t, "a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b", logSource.Config.Identifier)
}

func TestScheduleConfigCreatesNewService(t *testing.T) {
	logSources := config.NewLogSources()
	services := service.NewServices()
	scheduler := NewScheduler(logSources, services)

	servicesStream := services.GetAddedServices(service.Docker)

	configService := integration.Config{
		LogsConfig:   []byte(""),
		Entity:       "docker://a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b",
		ClusterCheck: false,
		CreationTime: 0,
	}

	go scheduler.Schedule([]integration.Config{configService})
	svc := <-servicesStream
	assert.Equal(t, configService.Entity, svc.GetEntityID())
}

func TestUnscheduleConfigRemovesSource(t *testing.T) {
	logSources := config.NewLogSources()
	services := service.NewServices()
	scheduler := NewScheduler(logSources, services)
	logSourcesStream := logSources.GetRemovedForType(config.DockerType)

	configSource := integration.Config{
		LogsConfig:    []byte(`[{"service":"foo","source":"bar"}]`),
		ADIdentifiers: []string{"docker://a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b"},
		Provider:      providers.Kubernetes,
		Entity:        "docker://a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b",
		ClusterCheck:  false,
		CreationTime:  0,
	}

	// We need to have a source to remove
	sources, _ := scheduler.toSources(configSource)
	logSources.AddSource(sources[0])

	go scheduler.Unschedule([]integration.Config{configSource})
	logSource := <-logSourcesStream
	assert.Equal(t, config.DockerType, logSource.Name)
	// We use the docker socket, not sourceType here
	assert.Equal(t, "", logSource.GetSourceType())
	assert.Equal(t, "foo", logSource.Config.Service)
	assert.Equal(t, "bar", logSource.Config.Source)
	assert.Equal(t, config.DockerType, logSource.Config.Type)
	assert.Equal(t, "a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b", logSource.Config.Identifier)
}

func TestUnscheduleConfigRemovesService(t *testing.T) {
	logSources := config.NewLogSources()
	services := service.NewServices()
	scheduler := NewScheduler(logSources, services)
	servicesStream := services.GetRemovedServices(service.Docker)

	configService := integration.Config{
		LogsConfig:   []byte(""),
		Entity:       "docker://a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b",
		ClusterCheck: false,
		CreationTime: 0,
	}

	go scheduler.Unschedule([]integration.Config{configService})
	svc := <-servicesStream
	assert.Equal(t, configService.Entity, svc.GetEntityID())
}
