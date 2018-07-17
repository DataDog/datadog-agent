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
	services   map[ID]Service
	newService chan<- Service
	delService chan<- Service
	ticker     *time.Ticker
	stop       chan bool
	health     *health.Handle
	m          sync.RWMutex
}

// PodContainerService implements and store results from the Service interface for the Kubelet listener
type PodContainerService struct {
	ID            ID
	ADIdentifiers []string
	Hosts         map[string]string
	Ports         []ContainerPort
}

func init() {
	Register("kubelet", NewKubeletListener)
}

func NewKubeletListener() (ServiceListener, error) {
	watcher, err := kubelet.NewPodWatcher(15 * time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to kubelet, Kubernetes listener will not work: %s", err)
	}
	return &KubeletListener{
		watcher:  watcher,
		services: make(map[ID]Service),
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
				for _, pod := range updatedPods {
					// Ignore pending/failed/succeeded/unknown states
					if pod.Status.Phase == "Running" {
						l.processNewPod(pod)
					}
				}
				// Compute deleted pods
				expiredContainerList, err := l.watcher.Expire()
				if err != nil {
					log.Error(err)
					continue
				}
				for _, containerID := range expiredContainerList {
					l.removeService(ID(containerID))
				}
			}
		}
	}()
}

func (l *KubeletListener) Stop() {
	l.ticker.Stop()
	l.stop <- true
}

func (l *KubeletListener) processNewPod(pod *kubelet.Pod) {
	for _, container := range pod.Status.Containers {
		l.createService(ID(container.ID), pod)
	}
}

func (l *KubeletListener) createService(id ID, pod *kubelet.Pod) {
	svc := PodContainerService{
		ID: id,
	}
	podName := pod.Metadata.Name

	// AD Identifiers
	var containerName string
	for _, container := range pod.Status.Containers {
		if container.ID == string(svc.ID) {
			containerName = container.Name

			// Add container uid as ID
			svc.ADIdentifiers = append(svc.ADIdentifiers, container.ID)

			// Stop here if we find an AD template annotation
			if podHasADTemplate(pod.Metadata.Annotations, containerName) {
				break
			}

			// Add other identifiers if no template found
			svc.ADIdentifiers = append(svc.ADIdentifiers, container.Image)
			_, short, _, err := containers.SplitImageName(container.Image)
			if err != nil {
				log.Warnf("Error while spliting image name: %s", err)
			}
			if len(short) > 0 && short != container.Image {
				svc.ADIdentifiers = append(svc.ADIdentifiers, short)
			}
			break
		}
	}

	// Hosts
	podIp := pod.Status.PodIP
	if podIp == "" {
		log.Errorf("Unable to get pod %s IP", podName)
	}
	svc.Hosts = map[string]string{"pod": podIp}

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
	svc.Ports = ports
	if len(svc.Ports) == 0 {
		// Port might not be specified in pod spec
		log.Debugf("No ports found for pod %s", podName)
	}

	l.m.Lock()
	l.services[ID(id)] = &svc
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

func (l *KubeletListener) removeService(cID ID) {
	if strings.HasPrefix(string(cID), kubelet.KubePodPrefix) {
		// Ignoring expired pods
		return
	}

	l.m.RLock()
	svc, ok := l.services[cID]
	l.m.RUnlock()

	if ok {
		l.m.Lock()
		delete(l.services, cID)
		l.m.Unlock()

		l.delService <- svc
	} else {
		log.Debugf("Container %s not found, not removing", cID)
	}
}

// GetID returns the service ID
func (s *PodContainerService) GetID() ID {
	return s.ID
}

// GetADIdentifiers returns the service AD identifiers
func (s *PodContainerService) GetADIdentifiers() ([]string, error) {
	return s.ADIdentifiers, nil
}

// GetHosts returns the pod hosts
func (s *PodContainerService) GetHosts() (map[string]string, error) {
	return s.Hosts, nil
}

// GetPid is not supported for PodContainerService
func (s *PodContainerService) GetPid() (int, error) {
	return -1, ErrNotSupported
}

// GetPorts returns the container's ports
func (s *PodContainerService) GetPorts() ([]ContainerPort, error) {
	return s.Ports, nil
}

// GetTags retrieves tags using the Tagger
func (s *PodContainerService) GetTags() ([]string, error) {
	return tagger.Tag(string(s.ID), tagger.IsFullCardinality())
}

// GetHostname returns nil and an error because port is not supported in Kubelet
func (s *PodContainerService) GetHostname() (string, error) {
	return "", ErrNotSupported
}
