// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package telemetry holds telemetry related files
package telemetry

import (
	"errors"
	"strings"

	"github.com/DataDog/datadog-go/v5/statsd"

	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmetafilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/util/workloadmeta"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/constants"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ContainersTelemetry represents the objects necessary to send metrics listing containers
type ContainersTelemetry struct {
	TelemetrySender SimpleTelemetrySender
	MetadataStore   workloadmeta.Component
	containerFilter workloadfilter.FilterBundle
}

// NewContainersTelemetry returns a new ContainersTelemetry based on default/global objects
func NewContainersTelemetry(telemetrySender SimpleTelemetrySender, wmeta workloadmeta.Component, containerFilter workloadfilter.FilterBundle) (*ContainersTelemetry, error) {
	errs := containerFilter.GetErrors()
	if errs != nil {
		return nil, errors.Join(errs...)
	}

	return &ContainersTelemetry{
		TelemetrySender: telemetrySender,
		MetadataStore:   wmeta,
		containerFilter: containerFilter,
	}, nil
}

// ListRunningContainers returns the list of running containers (from the workload meta store)
func (c *ContainersTelemetry) ListRunningContainers() []*workloadmeta.Container {
	return c.MetadataStore.ListContainersWithFilter(workloadmeta.GetRunningContainers)
}

// ReportContainers sends the metrics about currently running containers
// This function is critical for CWS/CSPM metering. Please tread carefully.
func (c *ContainersTelemetry) ReportContainers(metricName string) {
	containers := c.ListRunningContainers()

	for _, container := range containers {
		// ignore DD agent containers
		value := container.EnvVars["DOCKER_DD_AGENT"]
		value = strings.ToLower(value)

		pod, _ := c.MetadataStore.GetKubernetesPodForContainer(container.ID)
		filterablePod := workloadmetafilter.CreatePod(pod)
		filterableContainer := workloadmetafilter.CreateContainer(container, filterablePod)

		if (value == "yes" || value == "true") ||
			c.containerFilter.IsExcluded(filterableContainer) {
			log.Debugf("ignoring container: name=%s id=%s image_id=%s", container.Name, container.ID, container.Image.ID)
			continue
		}

		c.TelemetrySender.Gauge(metricName, 1.0, []string{"container_id:" + container.ID, constants.CardinalityTagPrefix + "orch"})
	}
	c.TelemetrySender.Commit()
}

// SimpleTelemetrySender is an abstraction over what is needed for the container telemetry
// the main goal is to be able to use it with a dogstatsd client or a SenderManager's default sender
type SimpleTelemetrySender interface {
	Gauge(name string, value float64, tags []string)
	Commit()
}

type statsdSTS struct {
	sci statsd.ClientInterface
}

func (s *statsdSTS) Gauge(name string, value float64, tags []string) {
	_ = s.sci.Gauge(name, value, tags, 1.0)
}

func (s *statsdSTS) Commit() {
	// nothing to do here
}

// NewSimpleTelemetrySenderFromStatsd returns a new SimpleTelemetrySender from a statsd client
func NewSimpleTelemetrySenderFromStatsd(sci statsd.ClientInterface) SimpleTelemetrySender {
	return &statsdSTS{
		sci: sci,
	}
}
