// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless
// +build !serverless

package listeners

import (
	"sort"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/common/utils"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

func init() {
	Register("kubelet", NewKubeletListener)
}

// KubeletListener listens to pod creation through a subscription
// to the workloadmeta store.
type KubeletListener struct {
	workloadmetaListener
}

// NewKubeletListener returns a new KubeletListener.
func NewKubeletListener(Config) (ServiceListener, error) {
	const name = "ad-kubeletlistener"

	l := &KubeletListener{}
	f := workloadmeta.NewFilter(
		[]workloadmeta.Kind{workloadmeta.KindKubernetesPod},
		workloadmeta.SourceNodeOrchestrator,
		workloadmeta.EventTypeAll,
	)

	var err error
	l.workloadmetaListener, err = newWorkloadmetaListener(name, f, l.processPod)
	if err != nil {
		return nil, err
	}

	return l, nil
}

func (l *KubeletListener) processPod(entity workloadmeta.Entity) {
	pod := entity.(*workloadmeta.KubernetesPod)

	containers := make([]*workloadmeta.Container, 0, len(pod.Containers))
	for _, podContainer := range pod.Containers {
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
	var ports []ContainerPort
	for _, container := range containers {
		for _, port := range container.Ports {
			ports = append(ports, ContainerPort{
				Port: port.Port,
				Name: port.Name,
			})
		}
	}

	sort.Slice(ports, func(i, j int) bool {
		return ports[i].Port < ports[j].Port
	})

	entity := kubelet.PodUIDToEntityName(pod.ID)
	svc := &service{
		entity:        pod,
		adIdentifiers: []string{entity},
		hosts:         map[string]string{"pod": pod.IP},
		ports:         ports,
		ready:         true,
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

	if l.IsExcluded(
		containers.GlobalFilter,
		pod.Annotations,
		containerName,
		containerImg.RawName,
		pod.Namespace,
	) {
		log.Debugf("container %s filtered out: name %q image %q namespace %q", container.ID, containerName, containerImg.RawName, pod.Namespace)
		return
	}

	// Note: Docker containers can have a "FinishedAt" time set even when
	// they're running. That happens when they've been stopped and then
	// restarted. "FinishedAt" corresponds to the last time the container was
	// stopped.
	if !container.State.Running && !container.State.FinishedAt.IsZero() {
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

	entity := containers.BuildEntityName(string(container.Runtime), container.ID)
	svc := &service{
		entity: container,
		ready:  pod.Ready,
		ports:  ports,
		extraConfig: map[string]string{
			"pod_name":  pod.Name,
			"namespace": pod.Namespace,
			"pod_uid":   pod.ID,
		},
		hosts: map[string]string{"pod": pod.IP},

		// Exclude non-running containers (including init containers)
		// from metrics collection but keep them for collecting logs.
		metricsExcluded: l.IsExcluded(
			containers.MetricsFilter,
			pod.Annotations,
			containerName,
			containerImg.RawName,
			pod.Namespace,
		) || !container.State.Running,
		logsExcluded: l.IsExcluded(
			containers.LogsFilter,
			pod.Annotations,
			containerName,
			containerImg.RawName,
			pod.Namespace,
		),
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
