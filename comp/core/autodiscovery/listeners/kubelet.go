// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package listeners

import (
	"errors"
	"sort"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/utils"
	"github.com/DataDog/datadog-agent/comp/core/tagger/common"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmetafilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/util/workloadmeta"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// KubeletListener listens to pod creation through a subscription
// to the workloadmeta store.
type KubeletListener struct {
	workloadmetaListener
	globalFilter  workloadfilter.FilterBundle
	metricsFilter workloadfilter.FilterBundle
	logsFilter    workloadfilter.FilterBundle
	tagger        tagger.Component
}

// NewKubeletListener returns a new KubeletListener.
func NewKubeletListener(options ServiceListernerDeps) (ServiceListener, error) {
	const name = "ad-kubeletlistener"

	l := &KubeletListener{
		globalFilter:  options.Filter.GetContainerAutodiscoveryFilters(workloadfilter.GlobalFilter),
		metricsFilter: options.Filter.GetContainerAutodiscoveryFilters(workloadfilter.MetricsFilter),
		logsFilter:    options.Filter.GetContainerAutodiscoveryFilters(workloadfilter.LogsFilter),
		tagger:        options.Tagger,
	}
	wmetaFilter := workloadmeta.NewFilterBuilder().
		SetSource(workloadmeta.SourceAll).
		AddKind(workloadmeta.KindKubernetesPod).
		Build()

	wmetaInstance, ok := options.Wmeta.Get()
	if !ok {
		return nil, errors.New("workloadmeta store is not initialized")
	}
	var err error
	l.workloadmetaListener, err = newWorkloadmetaListener(name, wmetaFilter, l.processPod, wmetaInstance, options.Telemetry)
	if err != nil {
		return nil, err
	}

	return l, nil
}

func (l *KubeletListener) processPod(entity workloadmeta.Entity) {
	// Fetch the pod from the workloadmeta store to get the most up-to-date state.
	// Handling cases where a pod deletion is reported as a 'Set' event due to
	// delayed updates from multiple workloadmeta sources. If the pod has been deleted,
	// its containers will be missing from the store, preventing stale container services
	// from being created.
	pod, err := l.Store().GetKubernetesPod(entity.GetID().ID)
	if err != nil || pod == nil {
		log.Debugf("Failed to get kubernetes pod from workloadmeta store, using pod from event")
		pod = entity.(*workloadmeta.KubernetesPod)
	}

	wlmContainers := pod.GetAllContainers()
	containers := make([]*workloadmeta.Container, 0, len(wlmContainers))
	for _, podContainer := range wlmContainers {
		container, err := l.Store().GetContainer(podContainer.ID)
		if err != nil {
			log.Debugf("pod %q has reference to non-existing container %q", pod.Name, podContainer.ID)
			continue
		}

		l.createContainerService(pod, &podContainer, container)

		containers = append(containers, container)
	}

	l.createPodService(pod, containers)
}

func (l *KubeletListener) createPodService(
	pod *workloadmeta.KubernetesPod,
	containers []*workloadmeta.Container,
) {
	var ports []workloadmeta.ContainerPort
	for _, container := range containers {
		for _, port := range container.Ports {
			ports = append(ports, workloadmeta.ContainerPort{
				Port: port.Port,
				Name: port.Name,
			})
		}
	}

	sort.Slice(ports, func(i, j int) bool {
		return ports[i].Port < ports[j].Port
	})

	entity := kubelet.PodUIDToEntityName(pod.ID)
	taggerEntityID := common.BuildTaggerEntityID(pod.GetID())
	svc := &WorkloadService{
		entity:        pod,
		tagsHash:      l.tagger.GetEntityHash(taggerEntityID, types.ChecksConfigCardinality),
		adIdentifiers: []string{entity},
		hosts:         map[string]string{"pod": pod.IP},
		ports:         ports,
		ready:         true,
		tagger:        l.tagger,
		wmeta:         l.Store(),
	}

	svcID := buildSvcID(pod.GetID())
	l.AddService(svcID, svc, "")
}

func (l *KubeletListener) createContainerService(
	pod *workloadmeta.KubernetesPod,
	podContainer *workloadmeta.OrchestratorContainer,
	container *workloadmeta.Container,
) {
	// we need to take the container name and image from the pod spec, as
	// the information from the container in the workloadmeta store might
	// have extra information resolved by the container runtime that won't
	// match what the user specified.
	containerName := podContainer.Name
	containerImg := podContainer.Image

	filterableContainer := workloadmetafilter.CreateContainerFromOrch(podContainer, workloadmetafilter.CreatePod(pod))

	if l.globalFilter.IsExcluded(filterableContainer) {
		log.Debugf("container %s filtered out: name %q image %q namespace %q", container.ID, containerName, containerImg.RawName, pod.Namespace)
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

	entity := containers.BuildEntityName(string(container.Runtime), container.ID)
	svc := &WorkloadService{
		entity:   container,
		tagsHash: l.tagger.GetEntityHash(types.NewEntityID(types.ContainerID, container.ID), types.ChecksConfigCardinality),
		ready:    pod.Ready || shouldSkipPodReadiness(pod),
		ports:    ports,
		extraConfig: map[string]string{
			"pod_name":  pod.Name,
			"namespace": pod.Namespace,
			"pod_uid":   pod.ID,
		},
		hosts: map[string]string{"pod": pod.IP},

		// Exclude non-running containers (including init containers)
		// from metrics collection but keep them for collecting logs.
		metricsExcluded: l.metricsFilter.IsExcluded(filterableContainer) || !container.State.Running,
		logsExcluded:    l.logsFilter.IsExcluded(filterableContainer),
		tagger:          l.tagger,
		imageName:       containerImg.ShortName,
		wmeta:           l.Store(),
	}

	adIdentifier := containerName
	if customADID, found := utils.ExtractCheckIDFromPodAnnotations(pod.Annotations, containerName); found {
		adIdentifier = customADID
		svc.adIdentifiers = append(svc.adIdentifiers, customADID)
	}

	svc.adIdentifiers = append(svc.adIdentifiers, entity, containerImg.RawName)

	if len(containerImg.ShortName) > 0 && containerImg.ShortName != containerImg.RawName {
		svc.adIdentifiers = append(svc.adIdentifiers, containerImg.ShortName)
	}

	var err error
	svc.checkNames, err = utils.ExtractCheckNamesFromPodAnnotations(pod.Annotations, adIdentifier)
	if err != nil {
		log.Error(err.Error())
	}

	svcID := buildSvcID(container.GetID())
	podSvcID := buildSvcID(pod.GetID())
	l.AddService(svcID, svc, podSvcID)
}
