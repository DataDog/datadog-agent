// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package listeners

import (
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/utils"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmetafilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/util/workloadmeta"
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
	globalFilter  workloadfilter.FilterBundle
	metricsFilter workloadfilter.FilterBundle
	logsFilter    workloadfilter.FilterBundle
	tagger        tagger.Component
}

// NewContainerListener returns a new ContainerListener.
func NewContainerListener(options ServiceListernerDeps) (ServiceListener, error) {
	const name = "ad-containerlistener"
	l := &ContainerListener{
		globalFilter:  options.Filter.GetContainerAutodiscoveryFilters(workloadfilter.GlobalFilter),
		metricsFilter: options.Filter.GetContainerAutodiscoveryFilters(workloadfilter.MetricsFilter),
		logsFilter:    options.Filter.GetContainerAutodiscoveryFilters(workloadfilter.LogsFilter),
		tagger:        options.Tagger,
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
	filterableContainer := workloadmetafilter.CreateContainer(container, workloadmetafilter.CreatePod(pod))

	if l.globalFilter.IsExcluded(filterableContainer) {
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

	ports := make([]workloadmeta.ContainerPort, 0, len(container.Ports))
	for _, port := range container.Ports {
		ports = append(ports, workloadmeta.ContainerPort{
			Port: port.Port,
			Name: port.Name,
		})
	}

	sort.Slice(ports, func(i, j int) bool {
		return ports[i].Port < ports[j].Port
	})

	svc := &WorkloadService{
		entity:   container,
		tagsHash: l.tagger.GetEntityHash(types.NewEntityID(types.ContainerID, container.ID), types.ChecksConfigCardinality),
		adIdentifiers: computeContainerServiceIDs(
			containers.BuildEntityName(string(container.Runtime), container.ID),
			containerImg.RawName,
			container.Labels,
		),
		ports:           ports,
		pid:             container.PID,
		hostname:        container.Hostname,
		metricsExcluded: l.metricsFilter.IsExcluded(filterableContainer),
		logsExcluded:    l.logsFilter.IsExcluded(filterableContainer),
		tagger:          l.tagger,
		wmeta:           l.Store(),
	}

	if pod != nil {
		svc.hosts = map[string]string{"pod": pod.IP}
		svc.ready = pod.Ready || shouldSkipPodReadiness(pod)

		adIdentifier := container.Name
		if customADID, found := utils.ExtractCheckIDFromPodAnnotations(pod.Annotations, container.Name); found {
			adIdentifier = customADID
			svc.adIdentifiers = append(svc.adIdentifiers, customADID)
		}

		checkNames, err := utils.ExtractCheckNamesFromPodAnnotations(pod.Annotations, adIdentifier)
		if err != nil {
			log.Errorf("error extracting check names from pod annotations: %s", err)
		}
		svc.checkNames = checkNames
	} else {
		checkNames, err := utils.ExtractCheckNamesFromContainerLabels(container.Labels)
		if err != nil {
			log.Errorf("error getting check names from labels on container %s: %v", container.ID, err)
		}

		svc.ready = true
		svc.hosts = docker.ContainerHosts(container.NetworkIPs, container.Labels, container.Hostname)
		svc.checkNames = checkNames
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
