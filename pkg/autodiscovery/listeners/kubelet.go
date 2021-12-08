// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless
// +build !serverless

package listeners

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/common/utils"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

const (
	newPodAnnotationFormat              = "ad.datadoghq.com/%s.instances"
	legacyPodAnnotationFormat           = "service-discovery.datadoghq.com/%s.instances"
	newPodAnnotationCheckNamesFormat    = "ad.datadoghq.com/%s.check_names"
	legacyPodAnnotationCheckNamesFormat = "service-discovery.datadoghq.com/%s.check_names"
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
func NewKubeletListener() (ServiceListener, error) {
	const name = "ad-kubeletlistener"

	l := &KubeletListener{}
	f := workloadmeta.NewFilter(
		[]workloadmeta.Kind{workloadmeta.KindKubernetesPod},
		[]workloadmeta.Source{workloadmeta.SourceKubelet},
	)

	var err error
	l.workloadmetaListener, err = newWorkloadmetaListener(name, f, l.processPod)
	if err != nil {
		return nil, err
	}

	return l, nil
}

func (l *KubeletListener) processPod(
	entity workloadmeta.Entity,
	creationTime integration.CreationTime,
) {
	pod := entity.(*workloadmeta.KubernetesPod)

	containers := make([]*workloadmeta.Container, 0, len(pod.Containers))
	for _, podContainer := range pod.Containers {
		container, err := l.Store().GetContainer(podContainer.ID)
		if err != nil {
			log.Debugf("pod %q has reference to non-existing container %q", pod.Name, podContainer.ID)
			continue
		}

		l.createContainerService(pod, &podContainer, container, creationTime)

		containers = append(containers, container)
	}

	l.createPodService(pod, containers, creationTime)
}

func (l *KubeletListener) createPodService(
	pod *workloadmeta.KubernetesPod,
	containers []*workloadmeta.Container,
	creationTime integration.CreationTime,
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
		creationTime:  creationTime,
		ready:         true,
	}

	svcID := buildSvcID(pod.GetID())
	l.AddService(svcID, svc, "")
}

func (l *KubeletListener) createContainerService(
	pod *workloadmeta.KubernetesPod,
	podContainer *workloadmeta.OrchestratorContainer,
	container *workloadmeta.Container,
	creationTime integration.CreationTime,
) {
	// we need to take the container name and image from the pod spec, as
	// the information from the container in the workloadmeta store might
	// have extra information resolved by the container runtime that won't
	// match what the user specified.
	containerName := podContainer.Name
	containerImg := podContainer.Image

	if l.IsExcluded(
		containers.GlobalFilter,
		containerName,
		containerImg.RawName,
		pod.Namespace,
	) {
		log.Debugf("container %s filtered out: name %q image %q namespace %q", container.ID, containerName, containerImg.RawName, pod.Namespace)
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

	entity := containers.BuildEntityName(string(container.Runtime), container.ID)
	svc := &service{
		entity:       container,
		creationTime: creationTime,
		ready:        pod.Ready,
		ports:        ports,
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
			containerName,
			containerImg.RawName,
			pod.Namespace,
		) || !container.State.Running,
		logsExcluded: l.IsExcluded(
			containers.LogsFilter,
			containerName,
			containerImg.RawName,
			pod.Namespace,
		),
	}

	adIdentifier := containerName

	// Check for custom AD identifiers
	if customADID, found := utils.GetCustomCheckID(pod.Annotations, containerName); found {
		adIdentifier = customADID
		svc.adIdentifiers = append(svc.adIdentifiers, customADID)
	}

	// Add container uid as ID
	svc.adIdentifiers = append(svc.adIdentifiers, entity)

	// Cache check names if the pod template is annotated
	if podHasADTemplate(pod.Annotations, adIdentifier) {
		var err error
		svc.checkNames, err = getCheckNamesFromAnnotations(pod.Annotations, adIdentifier)
		if err != nil {
			log.Error(err.Error())
		}
	}

	svc.adIdentifiers = append(svc.adIdentifiers, containerImg.RawName)

	if len(containerImg.ShortName) > 0 && containerImg.ShortName != containerImg.RawName {
		svc.adIdentifiers = append(svc.adIdentifiers, containerImg.ShortName)
	}

	svcID := buildSvcID(container.GetID())
	podSvcID := buildSvcID(pod.GetID())
	l.AddService(svcID, svc, podSvcID)
}

// podHasADTemplate looks in pod annotations and looks for annotations containing an
// AD template. It does not try to validate it, just having the `instance` fields is
// OK to return true.
func podHasADTemplate(annotations map[string]string, containerName string) bool {
	if _, found := annotations[fmt.Sprintf(newPodAnnotationFormat, containerName)]; found {
		return true
	}
	if _, found := annotations[fmt.Sprintf(legacyPodAnnotationFormat, containerName)]; found {
		return true
	}
	return false
}

// getCheckNamesFromAnnotations unmarshals the json string of check names
// defined in pod annotations and returns a slice of check names
func getCheckNamesFromAnnotations(annotations map[string]string, containerName string) ([]string, error) {
	if checkNamesJSON, found := annotations[fmt.Sprintf(newPodAnnotationCheckNamesFormat, containerName)]; found {
		checkNames := []string{}
		err := json.Unmarshal([]byte(checkNamesJSON), &checkNames)
		if err != nil {
			return nil, fmt.Errorf("Cannot parse check names: %v", err)
		}
		return checkNames, nil
	}
	if checkNamesJSON, found := annotations[fmt.Sprintf(legacyPodAnnotationCheckNamesFormat, containerName)]; found {
		checkNames := []string{}
		err := json.Unmarshal([]byte(checkNamesJSON), &checkNames)
		if err != nil {
			return nil, fmt.Errorf("Cannot parse check names: %v", err)
		}
		return checkNames, nil
	}
	return nil, nil
}
