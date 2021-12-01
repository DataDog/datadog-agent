// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build !serverless

package listeners

import (
	"sort"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

func init() {
	Register("container", NewContainerListener)
}

// ContainerListener listens to container creation through a subscription to the
// workloadmeta store.
type ContainerListener struct {
	workloadmetaListener
}

// NewContainerListener returns a new ContainerListener.
func NewContainerListener() (ServiceListener, error) {
	const name = "ad-containerlistener"
	l := &ContainerListener{}
	f := workloadmeta.NewFilter(
		[]workloadmeta.Kind{workloadmeta.KindContainer},
		[]workloadmeta.Source{workloadmeta.SourceDocker, workloadmeta.SourceContainerd, workloadmeta.SourcePodman},
	)

	var err error
	l.workloadmetaListener, err = newWorkloadmetaListener(name, f, l.createContainerService)
	if err != nil {
		return nil, err
	}

	return l, nil
}

func (l *ContainerListener) createContainerService(
	entity workloadmeta.Entity,
	creationTime integration.CreationTime,
) {
	container := entity.(*workloadmeta.Container)

	containerImg := container.Image
	if l.IsExcluded(
		containers.GlobalFilter,
		container.Name,
		containerImg.RawName,
		"",
	) {
		log.Debugf("container %s filtered out: name %q image %q", container.ID, container.Name, containerImg.RawName)
		return
	}

	if !container.State.FinishedAt.IsZero() {
		finishedAt := container.State.FinishedAt
		excludeAge := time.Duration(config.Datadog.GetInt("container_exclude_stopped_age")) * time.Hour
		if time.Now().Sub(finishedAt) > excludeAge {
			log.Debugf("container %q not running for too long, skipping", container.ID)
			return
		}
	}

	ports := make([]ContainerPort, 0, len(container.Ports))
	for _, port := range container.Ports {
		ports = append(ports, ContainerPort{
			Port: port.Port,
			Name: port.Name,
		})
	}

	sort.Slice(ports, func(i, j int) bool {
		return ports[i].Port < ports[j].Port
	})

	svc := &service{
		entity:       container,
		creationTime: integration.After,
		adIdentifiers: ComputeContainerServiceIDs(
			containers.BuildEntityName(string(container.Runtime), container.ID),
			containerImg.RawName,
			container.Labels,
		),
		ports:    ports,
		pid:      container.PID,
		hostname: container.Hostname,
	}

	if findKubernetesInLabels(container.Labels) {
		pod, err := l.Store().GetKubernetesPodForContainer(container.ID)
		if err == nil {
			svc.hosts = map[string]string{"pod": pod.IP}
			svc.ready = pod.Ready
		} else {
			log.Debugf("container %q belongs to a pod but was not found: %s", container.ID, err)
		}
	} else {
		checkNames, err := getCheckNamesFromLabels(container.Labels)
		if err != nil {
			log.Errorf("error getting check names from labels on container %s: %v", container.ID, err)
		}

		hosts := make(map[string]string)
		for host, ip := range container.NetworkIPs {
			hosts[host] = ip
		}

		if rancherIP, ok := docker.FindRancherIPInLabels(container.Labels); ok {
			hosts["rancher"] = rancherIP
		}

		// Some CNI solutions (including ECS awsvpc) do not assign an
		// IP through docker, but set a valid reachable hostname. Use
		// it if no IP is discovered.
		if len(hosts) == 0 && len(container.Hostname) > 0 {
			hosts["hostname"] = container.Hostname
		}

		svc.ready = true
		svc.hosts = hosts
		svc.checkNames = checkNames
		svc.metricsExcluded = l.IsExcluded(
			containers.MetricsFilter,
			container.Name,
			containerImg.RawName,
			"",
		)
		svc.logsExcluded = l.IsExcluded(
			containers.LogsFilter,
			container.Name,
			containerImg.RawName,
			"",
		)
	}

	svcID := buildSvcID(container.GetID())
	l.AddService(svcID, svc, "")
}

// findKubernetesInLabels traverses a map of container labels and
// returns true if a kubernetes label is detected
func findKubernetesInLabels(labels map[string]string) bool {
	for name := range labels {
		if strings.HasPrefix(name, "io.kubernetes.") {
			return true
		}
	}
	return false
}
