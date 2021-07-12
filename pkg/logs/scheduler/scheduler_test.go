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
	"github.com/stretchr/testify/assert"
)

func TestScheduleConfigCreatesNewSource(t *testing.T) {
	logSources := config.NewLogSources()
	services := service.NewServices()
	CreateScheduler(logSources, services)

	logSourcesStream := logSources.GetAddedForType(config.DockerType)

	configSource := integration.Config{
		LogsConfig:    []byte(`[{"service":"foo","source":"bar"}]`),
		ADIdentifiers: []string{"docker://a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b"},
		Provider:      names.Kubernetes,
		TaggerEntity:  "container_id://a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b",
		Entity:        "docker://a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b",
		ClusterCheck:  false,
		CreationTime:  0,
	}

	go adScheduler.Schedule([]integration.Config{configSource})
	logSource := <-logSourcesStream
	assert.Equal(t, config.DockerType, logSource.Name)
	// We use the docker socket, not sourceType here
	assert.Equal(t, config.SourceType(""), logSource.GetSourceType())
	assert.Equal(t, "foo", logSource.Config.Service)
	assert.Equal(t, "bar", logSource.Config.Source)
	assert.Equal(t, config.DockerType, logSource.Config.Type)
	assert.Equal(t, "a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b", logSource.Config.Identifier)
}

func TestScheduleConfigCreatesNewSourceServiceFallback(t *testing.T) {
	logSources := config.NewLogSources()
	services := service.NewServices()
	CreateScheduler(logSources, services)

	logSourcesStream := logSources.GetAddedForType(config.DockerType)

	configSource := integration.Config{
		InitConfig:    []byte(`{"service":"foo"}`),
		LogsConfig:    []byte(`[{"source":"bar"}]`),
		ADIdentifiers: []string{"docker://a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b"},
		Provider:      names.Kubernetes,
		TaggerEntity:  "container_id://a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b",
		Entity:        "docker://a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b",
		ClusterCheck:  false,
		CreationTime:  0,
	}

	go adScheduler.Schedule([]integration.Config{configSource})
	logSource := <-logSourcesStream
	assert.Equal(t, config.DockerType, logSource.Name)
	// We use the docker socket, not sourceType here
	assert.Equal(t, config.SourceType(""), logSource.GetSourceType())
	assert.Equal(t, "foo", logSource.Config.Service)
	assert.Equal(t, "bar", logSource.Config.Source)
	assert.Equal(t, config.DockerType, logSource.Config.Type)
	assert.Equal(t, "a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b", logSource.Config.Identifier)
}

func TestScheduleConfigCreatesNewSourceServiceOverride(t *testing.T) {
	logSources := config.NewLogSources()
	services := service.NewServices()
	CreateScheduler(logSources, services)

	logSourcesStream := logSources.GetAddedForType(config.DockerType)

	configSource := integration.Config{
		InitConfig:    []byte(`{"service":"foo"}`),
		LogsConfig:    []byte(`[{"source":"bar","service":"baz"}]`),
		ADIdentifiers: []string{"docker://a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b"},
		Provider:      names.Kubernetes,
		TaggerEntity:  "container_id://a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b",
		Entity:        "docker://a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b",
		ClusterCheck:  false,
		CreationTime:  0,
	}

	go adScheduler.Schedule([]integration.Config{configSource})
	logSource := <-logSourcesStream
	assert.Equal(t, config.DockerType, logSource.Name)
	// We use the docker socket, not sourceType here
	assert.Equal(t, config.SourceType(""), logSource.GetSourceType())
	assert.Equal(t, "baz", logSource.Config.Service)
	assert.Equal(t, "bar", logSource.Config.Source)
	assert.Equal(t, config.DockerType, logSource.Config.Type)
	assert.Equal(t, "a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b", logSource.Config.Identifier)
}

func TestScheduleConfigCreatesNewService(t *testing.T) {
	logSources := config.NewLogSources()
	services := service.NewServices()
	CreateScheduler(logSources, services)

	servicesStream := services.GetAddedServicesForType(config.DockerType)

	configService := integration.Config{
		LogsConfig:   []byte(""),
		TaggerEntity: "container_id://a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b",
		Entity:       "docker://a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b",
		ClusterCheck: false,
		CreationTime: 0,
	}

	go adScheduler.Schedule([]integration.Config{configService})
	svc := <-servicesStream
	assert.Equal(t, configService.Entity, svc.GetEntityID())

	// shouldn't consider pods
	configService = integration.Config{
		LogsConfig:   []byte(""),
		TaggerEntity: "kubernetes_pod://ee9a4083-10fc-11ea-a545-02c6fa0ccfb0",
		Entity:       "kubernetes_pod://ee9a4083-10fc-11ea-a545-02c6fa0ccfb0",
		ClusterCheck: false,
		CreationTime: 0,
	}
	go adScheduler.Schedule([]integration.Config{configService})
	select {
	case <-servicesStream:
		assert.Fail(t, "Pod should be ignored")
	case <-time.After(100 * time.Millisecond):
		break
	}
}

func TestUnscheduleConfigRemovesSource(t *testing.T) {
	logSources := config.NewLogSources()
	services := service.NewServices()
	CreateScheduler(logSources, services)
	logSourcesStream := logSources.GetRemovedForType(config.DockerType)

	configSource := integration.Config{
		LogsConfig:    []byte(`[{"service":"foo","source":"bar"}]`),
		ADIdentifiers: []string{"docker://a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b"},
		Provider:      names.Kubernetes,
		TaggerEntity:  "container_id://a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b",
		Entity:        "docker://a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b",
		ClusterCheck:  false,
		CreationTime:  0,
	}

	// We need to have a source to remove
	sources, _ := adScheduler.toSources(configSource)
	logSources.AddSource(sources[0])

	go adScheduler.Unschedule([]integration.Config{configSource})
	logSource := <-logSourcesStream
	assert.Equal(t, config.DockerType, logSource.Name)
	// We use the docker socket, not sourceType here
	assert.Equal(t, config.SourceType(""), logSource.GetSourceType())
	assert.Equal(t, "foo", logSource.Config.Service)
	assert.Equal(t, "bar", logSource.Config.Source)
	assert.Equal(t, config.DockerType, logSource.Config.Type)
	assert.Equal(t, "a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b", logSource.Config.Identifier)
}

func TestUnscheduleConfigRemovesService(t *testing.T) {
	logSources := config.NewLogSources()
	services := service.NewServices()
	CreateScheduler(logSources, services)
	servicesStream := services.GetRemovedServicesForType(config.DockerType)

	configService := integration.Config{
		LogsConfig:   []byte(""),
		TaggerEntity: "container_id://a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b",
		Entity:       "docker://a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b",
		ClusterCheck: false,
		CreationTime: 0,
	}

	go adScheduler.Unschedule([]integration.Config{configService})
	svc := <-servicesStream
	assert.Equal(t, configService.Entity, svc.GetEntityID())

	// shouldn't consider pods
	configService = integration.Config{
		LogsConfig:   []byte(""),
		TaggerEntity: "kubernetes_pod://ee9a4083-10fc-11ea-a545-02c6fa0ccfb0",
		Entity:       "kubernetes_pod://ee9a4083-10fc-11ea-a545-02c6fa0ccfb0",
		ClusterCheck: false,
		CreationTime: 0,
	}

	go adScheduler.Unschedule([]integration.Config{configService})
	select {
	case <-servicesStream:
		assert.Fail(t, "Pod should be ignored")
	case <-time.After(100 * time.Millisecond):
		break
	}
}

func TestIgnoreConfigIfLogsExcluded(t *testing.T) {
	logSources := config.NewLogSources()
	services := service.NewServices()
	CreateScheduler(logSources, services)
	servicesStreamIn := services.GetAddedServicesForType(config.DockerType)
	servicesStreamOut := services.GetRemovedServicesForType(config.DockerType)

	configService := integration.Config{
		LogsConfig:   []byte(""),
		TaggerEntity: "container_id://a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b",
		Entity:       "docker://a1887023ed72a2b0d083ef465e8edfe4932a25731d4bda2f39f288f70af3405b",
		ClusterCheck: false,
		CreationTime: 0,
		LogsExcluded: true,
	}

	go adScheduler.Schedule([]integration.Config{configService})
	select {
	case <-servicesStreamIn:
		assert.Fail(t, "config must be ignored")
	case <-time.After(100 * time.Millisecond):
		break
	}

	go adScheduler.Unschedule([]integration.Config{configService})
	select {
	case <-servicesStreamOut:
		assert.Fail(t, "config must be ignored")
	case <-time.After(100 * time.Millisecond):
		break
	}
}
