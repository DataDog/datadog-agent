// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package listeners

import (
	"sort"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/common/utils"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

const (
	newIdentifierLabel    = "com.datadoghq.ad.check.id"
	legacyIdentifierLabel = "com.datadoghq.sd.check.id"
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
func NewContainerListener(Config) (ServiceListener, error) {
	const name = "ad-containerlistener"
	l := &ContainerListener{}
	f := workloadmeta.NewFilter(
		[]workloadmeta.Kind{workloadmeta.KindContainer},
		workloadmeta.SourceRuntime,
		workloadmeta.EventTypeAll,
	)

	var err error
	l.workloadmetaListener, err = newWorkloadmetaListener(name, f, l.createContainerService)
	if err != nil {
		return nil, err
	}

	return l, nil
}

func (l *ContainerListener) createContainerService(entity workloadmeta.Entity) {
	container := entity.(*workloadmeta.Container)
	var annotations map[string]string
	var pod *workloadmeta.KubernetesPod
	if findKubernetesInLabels(container.Labels) {
		kubePod, err := l.Store().GetKubernetesPodForContainer(container.ID)
		if err == nil {
			pod = kubePod
			annotations = pod.Annotations
		} else {
			log.Debugf("container %q belongs to a pod but was not found: %s", container.ID, err)
		}
	}
	containerImg := container.Image
	if l.IsExcluded(
		containers.GlobalFilter,
		annotations,
		container.Name,
		containerImg.RawName,
		"",
	) {
		log.Debugf("container %s filtered out: name %q image %q", container.ID, container.Name, containerImg.RawName)
		return
	}

	// Note: Docker containers can have a "FinishedAt" time set even when
	// they're running. That happens when they've been stopped and then
	// restarted. "FinishedAt" corresponds to the last time the container was
	// stopped.
	if !container.State.Running && !container.State.FinishedAt.IsZero() {
		finishedAt := container.State.FinishedAt
		excludeAge := time.Duration(config.Datadog.GetInt("container_exclude_stopped_age")) * time.Hour
		if time.Since(finishedAt) > excludeAge {
			log.Debugf("container %q not running for too long, skipping", container.ID)
			return
		}
	}

	if !container.State.Running && container.Runtime == workloadmeta.ContainerRuntimeECSFargate {
		return
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
		entity: container,
		adIdentifiers: computeContainerServiceIDs(
			containers.BuildEntityName(string(container.Runtime), container.ID),
			containerImg.RawName,
			container.Labels,
		),
		ports:    ports,
		pid:      container.PID,
		hostname: container.Hostname,
	}

	if pod != nil {
		svc.hosts = map[string]string{"pod": pod.IP}
		svc.ready = pod.Ready

		svc.metricsExcluded = l.IsExcluded(
			containers.MetricsFilter,
			pod.Annotations,
			container.Name,
			containerImg.RawName,
			"",
		)
		svc.logsExcluded = l.IsExcluded(
			containers.LogsFilter,
			pod.Annotations,
			container.Name,
			containerImg.RawName,
			"",
		)
	} else {
		checkNames, err := utils.ExtractCheckNamesFromContainerLabels(container.Labels)
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
			nil,
			container.Name,
			containerImg.RawName,
			"",
		)
		svc.logsExcluded = l.IsExcluded(
			containers.LogsFilter,
			nil,
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

// computeContainerServiceIDs takes an entity name, an image (resolved to an
// actual name) and labels and computes the service IDs for this container
// service.
func computeContainerServiceIDs(entity string, image string, labels map[string]string) []string {
	// ID override label
	if l, found := labels[newIdentifierLabel]; found {
		return []string{l}
	}
	if l, found := labels[legacyIdentifierLabel]; found {
		log.Warnf("found legacy %s label for %s, please use the new name %s",
			legacyIdentifierLabel, entity, newIdentifierLabel)
		return []string{l}
	}

	ids := []string{entity}

	// Add Image names (long then short if different)
	long, _, short, _, err := containers.SplitImageName(image)
	if err != nil {
		log.Warnf("error while spliting image name: %s", err)
	}
	if len(long) > 0 {
		ids = append(ids, long)
	}
	if len(short) > 0 && short != long {
		ids = append(ids, short)
	}
	return ids
}
