// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build kubelet

package listeners

import (
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	log "github.com/cihub/seelog"
)

// KubeletListener listen to kubelet pod creation
type KubeletListener struct {
	watcher    *kubelet.PodWatcher
	services   map[ID]Service
	newService chan<- Service
	delService chan<- Service
	ticker     *time.Ticker
	stop       chan bool
	m          sync.RWMutex
}

// PodService implements and store results from the Service interface for the Kubelet listener
type PodService struct {
	ID            ID
	ADIdentifiers []string
	Hosts         map[string]string
	Ports         []int
	Pid           int
}

func init() {
	Register("kubelet", NewKubeletListener)
}

func NewKubeletListener() (ServiceListener, error) {
	watcher, err := kubelet.NewPodWatcher()
	if err != nil {
		return nil, fmt.Errorf("Failed to connect to kubelet, Kubernetes listener will not work: %s", err)
	}
	return &KubeletListener{
		watcher:  watcher,
		services: make(map[ID]Service),
		ticker:   time.NewTicker(time.Second * 5),
		stop:     make(chan bool),
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
				return
			case <-l.ticker.C:
				log.Error("yolo")
				// Compute new/updated pods
				updatedPods, err := l.watcher.PullChanges()
				if err != nil {
					log.Error(err)
				}
				log.Debug(updatedPods)
			}
		}
	}()
}

func (l *KubeletListener) Stop() {
	l.ticker.Stop()
	l.stop <- true
}

func (l *KubeletListener) createService() {
	svc := PodService{}

	l.newService <- &svc
}

// GetID returns the service ID
func (s *PodService) GetID() ID {
	return s.ID
}

// GetADIdentifiers returns the service AD identifiers
func (s *PodService) GetADIdentifiers() ([]string, error) {
	if len(s.ADIdentifiers) == 0 {
		// get image names from pod
	}

	return s.ADIdentifiers, nil
}

// GetHosts returns the pod hosts
func (s *PodService) GetHosts() (map[string]string, error) {
	return s.Hosts, nil
}

// GetPid inspect the container an return its pid
func (s *PodService) GetPid() (int, error) {
	return s.Pid, nil
}

// GetPorts returns the container's ports
func (s *PodService) GetPorts() ([]int, error) {
	return s.Ports, nil
}

// GetTags retrieves tags using the Tagger
func (s *PodService) GetTags() ([]string, error) {
	return []string{}, nil
}
