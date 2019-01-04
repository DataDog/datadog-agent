// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubelet

package listeners

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	newPodAnnotationFormat    = "ad.datadoghq.com/%s.instances"
	legacyPodAnnotationFormat = "service-discovery.datadoghq.com/%s.instances"
)

// KubeletListener listen to kubelet pod creation
type KubeletListener struct {
	watcher    *kubelet.PodWatcher
	filter     *containers.Filter
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
	entity        string
	adIdentifiers []string
	hosts         map[string]string
	ports         []ContainerPort
	creationTime  integration.CreationTime
}

func init() {
	Register("kubelet", NewKubeletListener)
}

func NewKubeletListener() (ServiceListener, error) {
	watcher, err := kubelet.NewPodWatcher(15 * time.Second)
	if err != nil {
		return nil, err
	}
	filter, err := containers.NewFilterFromConfigIncludePause()
	if err != nil {
		return nil, err
	}
	return &KubeletListener{
		watcher:  watcher,
		filter:   filter,
		services: make(map[string]Service),
		ticker:   time.NewTicker(15 * time.Second),
		stop:     make(chan bool),
		health:   health.Register("ad-kubeletlistener"),
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
				l.health.Deregister()
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
		// Ignore pending/failed/succeeded/unknown states
		if pod.Status.Phase == "Running" {
			for _, container := range pod.Status.Containers {
				l.createService(container.ID, pod, firstRun)
			}
		}
	}
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
	}
	podName := pod.Metadata.Name

	// AD Identifiers
	var containerName string
	for _, container := range pod.Status.Containers {
		if container.ID == svc.entity {
			if l.filter.IsExcluded(container.Name, container.Image) {
				log.Debugf("container %s filtered out: name %q image %q", container.ID, container.Name, container.Image)
				return
			}
			containerName = container.Name

			// Add container uid as ID
			svc.adIdentifiers = append(svc.adIdentifiers, container.ID)

			// Stop here if we find an AD template annotation
			if podHasADTemplate(pod.Metadata.Annotations, containerName) {
				break
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
	podIp := pod.Status.PodIP
	if podIp == "" {
		log.Errorf("Unable to get pod %s IP", podName)
	}
	svc.hosts = map[string]string{"pod": podIp}

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

func (l *KubeletListener) removeService(entity string) {
	if strings.HasPrefix(entity, kubelet.KubePodPrefix) {
		// Ignoring expired pods
		return
	}

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
	return tagger.Tag(string(s.entity), tagger.IsChecksFullCardinality())
}

// GetHostname returns nil and an error because port is not supported in Kubelet
func (s *KubeContainerService) GetHostname() (string, error) {
	return "", ErrNotSupported
}

// GetCreationTime returns the creation time of the container compare to the agent start.
func (s *KubeContainerService) GetCreationTime() integration.CreationTime {
	return s.creationTime
}
