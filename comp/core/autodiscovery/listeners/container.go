// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package listeners

import (
	"errors"
	"maps"
	"sort"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/utils"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	filter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	pkgcontainersimage "github.com/DataDog/datadog-agent/pkg/util/containers/image"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	newIdentifierLabel    = "com.datadoghq.ad.check.id"
	legacyIdentifierLabel = "com.datadoghq.sd.check.id"
)

// ContainerListener listens to container creation through a subscription to the
// workloadmeta store.
type ContainerListener struct {
	workloadmetaListener
	filterStore filter.Component
	tagger      tagger.Component
}

// NewContainerListener returns a new ContainerListener.
func NewContainerListener(options ServiceListernerDeps) (ServiceListener, error) {
	const name = "ad-containerlistener"
	l := &ContainerListener{
		filterStore: options.Filter,
		tagger:      options.Tagger,
	}
	filter := workloadmeta.NewFilterBuilder().
		SetSource(workloadmeta.SourceAll).
		AddKind(workloadmeta.KindContainer).Build()

	wmetaInstance, ok := options.Wmeta.Get()
	if !ok {
		return nil, errors.New("workloadmeta store is not initialized")
	}
	var err error
	l.workloadmetaListener, err = newWorkloadmetaListener(name, filter, l.createContainerService, wmetaInstance, options.Telemetry)
	if err != nil {
		return nil, err
	}

	return l, nil
}

func (l *ContainerListener) createContainerService(entity workloadmeta.Entity) {
	container := entity.(*workloadmeta.Container)
	var pod *workloadmeta.KubernetesPod
	if findKubernetesInLabels(container.Labels) {
		kubePod, err := l.Store().GetKubernetesPodForContainer(container.ID)
		if err == nil {
			pod = kubePod
		} else {
			log.Debugf("container %q belongs to a pod but was not found: %s", container.ID, err)
		}
	}
	containerImg := container.Image
	if l.filterStore.IsContainerExcluded(
		filter.CreateContainer(container, filter.CreatePod(pod)),
		filter.GetAutodiscoveryFilters(filter.GlobalFilter),
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
		excludeAge := time.Duration(pkgconfigsetup.Datadog().GetInt("container_exclude_stopped_age")) * time.Hour
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
		entity:   container,
		tagsHash: l.tagger.GetEntityHash(types.NewEntityID(types.ContainerID, container.ID), types.ChecksConfigCardinality),
		adIdentifiers: computeContainerServiceIDs(
			containers.BuildEntityName(string(container.Runtime), container.ID),
			containerImg.RawName,
			container.Labels,
		),
		ports:    ports,
		pid:      container.PID,
		hostname: container.Hostname,
		tagger:   l.tagger,
	}

	if pod != nil {
		svc.hosts = map[string]string{"pod": pod.IP}
		svc.ready = pod.Ready

		svc.metricsExcluded = l.filterStore.IsContainerExcluded(
			filter.CreateContainer(container, filter.CreatePod(pod)),
			filter.GetAutodiscoveryFilters(filter.MetricsFilter),
		)
		svc.logsExcluded = l.filterStore.IsContainerExcluded(
			filter.CreateContainer(container, filter.CreatePod(pod)),
			filter.GetAutodiscoveryFilters(filter.LogsFilter),
		)
	} else {
		checkNames, err := utils.ExtractCheckNamesFromContainerLabels(container.Labels)
		if err != nil {
			log.Errorf("error getting check names from labels on container %s: %v", container.ID, err)
		}

		hosts := make(map[string]string)
		maps.Copy(hosts, container.NetworkIPs)

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
		svc.metricsExcluded = l.filterStore.IsContainerExcluded(
			filter.CreateContainer(container, nil),
			filter.GetAutodiscoveryFilters(filter.MetricsFilter),
		)
		svc.logsExcluded = l.filterStore.IsContainerExcluded(
			filter.CreateContainer(container, nil),
			filter.GetAutodiscoveryFilters(filter.LogsFilter),
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
	long, _, short, _, err := pkgcontainersimage.SplitImageName(image)
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
