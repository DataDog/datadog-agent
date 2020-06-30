// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubelet

package listeners

import (
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	newPodAnnotationFormat              = "ad.datadoghq.com/%s.instances"
	legacyPodAnnotationFormat           = "service-discovery.datadoghq.com/%s.instances"
	newPodAnnotationCheckNamesFormat    = "ad.datadoghq.com/%s.check_names"
	legacyPodAnnotationCheckNamesFormat = "service-discovery.datadoghq.com/%s.check_names"
)

// KubeletListener listen to kubelet pod creation
type KubeletListener struct {
	watcher    *kubelet.PodWatcher
	filters    *containerFilters
	services   map[string]Service
	newService chan<- Service
	delService chan<- Service
	ticker     *time.Ticker
	stop       chan bool
	health     *health.Handle
	m          sync.RWMutex
}

// KubeContainerService implements and store results from the Service interface for the Kubelet listener
type KubeContainerService struct {
	entity          string
	adIdentifiers   []string
	hosts           map[string]string
	ports           []ContainerPort
	creationTime    integration.CreationTime
	ready           bool
	checkNames      []string
	metricsExcluded bool
	logsExcluded    bool
}

// Make sure KubeContainerService implements the Service interface
var _ Service = &KubeContainerService{}

// KubePodService registers pod as a Service, implements and store results from the Service interface for the Kubelet listener
// needed to run checks on pod's endpoints
type KubePodService struct {
	entity        string
	adIdentifiers []string
	hosts         map[string]string
	ports         []ContainerPort
	creationTime  integration.CreationTime
}

// Make sure KubePodService implements the Service interface
var _ Service = &KubePodService{}

func init() {
	Register("kubelet", NewKubeletListener)
}

func NewKubeletListener() (ServiceListener, error) {
	watcher, err := kubelet.NewPodWatcher(15*time.Second, false)
	if err != nil {
		return nil, err
	}
	filters, err := newContainerFilters()
	if err != nil {
		return nil, err
	}
	return &KubeletListener{
		watcher:  watcher,
		filters:  filters,
		services: make(map[string]Service),
		ticker:   time.NewTicker(config.Datadog.GetDuration("kubelet_listener_polling_interval") * time.Second),
		stop:     make(chan bool),
		health:   health.RegisterLiveness("ad-kubeletlistener"),
	}, nil
}

func (l *KubeletListener) Listen(newSvc chan<- Service, delSvc chan<- Service) {
	// setup the I/O channels
	l.newService = newSvc
	l.delService = delSvc

	go func() {
		pods, err := l.watcher.PullChanges()
		if err != nil {
			log.Error(err)
		}
		l.processNewPods(pods, true)

		for {
			select {
			case <-l.stop:
				l.health.Deregister() //nolint:errcheck
				return
			case <-l.health.C:
			case <-l.ticker.C:
				// Compute new/updated pods
				updatedPods, err := l.watcher.PullChanges()
				if err != nil {
					log.Error(err)
					continue
				}
				l.processNewPods(updatedPods, false)
				// Compute deleted pods
				expiredContainerList, err := l.watcher.Expire()
				if err != nil {
					log.Error(err)
					continue
				}
				for _, entity := range expiredContainerList {
					l.removeService(entity)
				}
			}
		}
	}()
}

func (l *KubeletListener) Stop() {
	l.ticker.Stop()
	l.stop <- true
}

func (l *KubeletListener) processNewPods(pods []*kubelet.Pod, firstRun bool) {
	for _, pod := range pods {
		// We ignore the state of the pod but only taking containers with ids
		// into consideration (not pending)
		for _, container := range pod.Status.GetAllContainers() {
			if !container.IsPending() {
				l.createService(container.ID, pod, firstRun)
			}
		}
		l.createPodService(pod, firstRun)
	}
}

func (l *KubeletListener) createPodService(pod *kubelet.Pod, firstRun bool) {
	var crTime integration.CreationTime
	if firstRun {
		crTime = integration.Before
	} else {
		crTime = integration.After
	}

	// Entity, to be used as an AD identifier too
	entity := kubelet.PodUIDToEntityName(pod.Metadata.UID)

	// Hosts
	podIP := pod.Status.PodIP
	if podIP == "" {
		log.Errorf("Unable to get pod %s IP", pod.Metadata.Name)
	}

	// Ports: adding all ports of pod's containers
	var ports []ContainerPort
	for _, container := range pod.Spec.Containers {
		for _, port := range container.Ports {
			ports = append(ports, ContainerPort{port.ContainerPort, port.Name})
		}
	}
	sort.Slice(ports, func(i, j int) bool {
		return ports[i].Port < ports[j].Port
	})

	if len(ports) == 0 {
		// Port might not be specified in pod spec
		log.Debugf("No ports found for pod %s", pod.Metadata.Name)
	}

	svc := KubePodService{
		entity:        entity,
		adIdentifiers: []string{entity},
		hosts:         map[string]string{"pod": podIP},
		ports:         ports,
		creationTime:  crTime,
	}

	l.m.Lock()
	l.services[entity] = &svc
	l.m.Unlock()

	l.newService <- &svc
}

func (l *KubeletListener) createService(entity string, pod *kubelet.Pod, firstRun bool) {
	var crTime integration.CreationTime
	if firstRun {
		crTime = integration.Before
	} else {
		crTime = integration.After
	}
	svc := KubeContainerService{
		entity:       entity,
		creationTime: crTime,
		ready:        kubelet.IsPodReady(pod),
	}
	podName := pod.Metadata.Name

	// AD Identifiers
	var containerName string
	for _, container := range pod.Status.GetAllContainers() {
		if container.ID == svc.entity {
			// Detect AD exclusion
			if l.filters.IsExcluded(containers.GlobalFilter, container.Name, container.Image, pod.Metadata.Namespace) {
				log.Debugf("container %s filtered out: name %q image %q namespace %q", container.ID, container.Name, container.Image, pod.Metadata.Namespace)
				return
			}

			// Detect metrics or logs exclusion
			svc.metricsExcluded = l.filters.IsExcluded(containers.MetricsFilter, container.Name, container.Image, pod.Metadata.Namespace)
			svc.logsExcluded = l.filters.IsExcluded(containers.LogsFilter, container.Name, container.Image, pod.Metadata.Namespace)

			containerName = container.Name

			// Add container uid as ID
			svc.adIdentifiers = append(svc.adIdentifiers, entity)

			// Cache check names if the pod template is annotated
			if podHasADTemplate(pod.Metadata.Annotations, containerName) {
				var err error
				svc.checkNames, err = getCheckNamesFromAnnotations(pod.Metadata.Annotations, containerName)
				if err != nil {
					log.Error(err.Error())
				}
			}

			// Add other identifiers if no template found
			svc.adIdentifiers = append(svc.adIdentifiers, container.Image)
			_, short, _, err := containers.SplitImageName(container.Image)
			if err != nil {
				log.Warnf("Error while spliting image name: %s", err)
			}
			if len(short) > 0 && short != container.Image {
				svc.adIdentifiers = append(svc.adIdentifiers, short)
			}
			break
		}
	}

	// Hosts
	podIP := pod.Status.PodIP
	if podIP == "" {
		log.Errorf("Unable to get pod %s IP", podName)
	}
	svc.hosts = map[string]string{"pod": podIP}

	// Ports
	var ports []ContainerPort
	for _, container := range pod.Spec.Containers {
		if container.Name == containerName {
			for _, port := range container.Ports {
				ports = append(ports, ContainerPort{port.ContainerPort, port.Name})
			}
			break
		}
	}
	sort.Slice(ports, func(i, j int) bool {
		return ports[i].Port < ports[j].Port
	})
	svc.ports = ports
	if len(svc.ports) == 0 {
		// Port might not be specified in pod spec
		log.Debugf("No ports found for pod %s", podName)
	}

	l.m.Lock()
	l.services[entity] = &svc
	l.m.Unlock()

	l.newService <- &svc
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

func (l *KubeletListener) removeService(entity string) {
	l.m.RLock()
	svc, ok := l.services[entity]
	l.m.RUnlock()

	if ok {
		l.m.Lock()
		delete(l.services, entity)
		l.m.Unlock()

		l.delService <- svc
	} else {
		log.Debugf("Entity %s not found, not removing", entity)
	}
}

// GetEntity returns the unique entity name linked to that service
func (s *KubeContainerService) GetEntity() string {
	return s.entity
}

// GetEntity returns the unique entity name linked to that service
func (s *KubeContainerService) GetTaggerEntity() string {
	taggerEntity, err := kubelet.KubeContainerIDToTaggerEntityID(s.entity)
	if err != nil {
		return s.entity
	}
	return taggerEntity
}

// GetADIdentifiers returns the service AD identifiers
func (s *KubeContainerService) GetADIdentifiers() ([]string, error) {
	return s.adIdentifiers, nil
}

// GetHosts returns the pod hosts
func (s *KubeContainerService) GetHosts() (map[string]string, error) {
	return s.hosts, nil
}

// GetPid is not supported for PodContainerService
func (s *KubeContainerService) GetPid() (int, error) {
	return -1, ErrNotSupported
}

// GetPorts returns the container's ports
func (s *KubeContainerService) GetPorts() ([]ContainerPort, error) {
	return s.ports, nil
}

// GetTags retrieves tags using the Tagger
func (s *KubeContainerService) GetTags() ([]string, error) {
	return tagger.Tag(s.GetTaggerEntity(), tagger.ChecksCardinality)
}

// GetHostname returns nil and an error because port is not supported in Kubelet
func (s *KubeContainerService) GetHostname() (string, error) {
	return "", ErrNotSupported
}

// GetCreationTime returns the creation time of the container compare to the agent start.
func (s *KubeContainerService) GetCreationTime() integration.CreationTime {
	return s.creationTime
}

// IsReady returns if the service is ready
func (s *KubeContainerService) IsReady() bool {
	return s.ready
}

// GetExtraConfig isn't supported
func (s *KubeContainerService) GetExtraConfig(key []byte) ([]byte, error) {
	return []byte{}, ErrNotSupported
}

// GetCheckNames returns names of checks defined in pod annotations
func (s *KubeContainerService) GetCheckNames() []string {
	return s.checkNames
}

// HasFilter returns true if metrics or logs collection must be excluded for this service
// no containers.GlobalFilter case here because we don't create services that are globally excluded in AD
func (s *KubeContainerService) HasFilter(filter containers.FilterType) bool {
	switch filter {
	case containers.MetricsFilter:
		return s.metricsExcluded
	case containers.LogsFilter:
		return s.logsExcluded
	}
	return false
}

// GetEntity returns the unique entity name linked to that service
func (s *KubePodService) GetEntity() string {
	return s.entity
}

// GetEntity returns the unique entity name linked to that service
func (s *KubePodService) GetTaggerEntity() string {
	taggerEntity, err := kubelet.KubePodUIDToTaggerEntityID(s.entity)
	if err != nil {
		return s.entity
	}
	return taggerEntity
}

// GetADIdentifiers returns the service AD identifiers
func (s *KubePodService) GetADIdentifiers() ([]string, error) {
	return s.adIdentifiers, nil
}

// GetHosts returns the pod hosts
func (s *KubePodService) GetHosts() (map[string]string, error) {
	return s.hosts, nil
}

// GetPid is not supported for PodContainerService
func (s *KubePodService) GetPid() (int, error) {
	return -1, ErrNotSupported
}

// GetPorts returns the container's ports
func (s *KubePodService) GetPorts() ([]ContainerPort, error) {
	return s.ports, nil
}

// GetTags retrieves tags using the Tagger
func (s *KubePodService) GetTags() ([]string, error) {
	return tagger.Tag(s.GetTaggerEntity(), tagger.ChecksCardinality)
}

// GetHostname returns nil and an error because port is not supported in Kubelet
func (s *KubePodService) GetHostname() (string, error) {
	return "", ErrNotSupported
}

// GetCreationTime returns the creation time of the container compare to the agent start.
func (s *KubePodService) GetCreationTime() integration.CreationTime {
	return s.creationTime
}

// IsReady returns if the service is ready
func (s *KubePodService) IsReady() bool {
	return true
}

// GetCheckNames returns slice of check names defined in kubernetes annotations or docker labels
// KubePodService doesn't implement this method
func (s *KubePodService) GetCheckNames() []string {
	return nil
}

// HasFilter always return false
// KubePodService doesn't implement this method
func (s *KubePodService) HasFilter(filter containers.FilterType) bool {
	return false
}

// GetExtraConfig isn't supported
func (s *KubePodService) GetExtraConfig(key []byte) ([]byte, error) {
	return []byte{}, ErrNotSupported
}
