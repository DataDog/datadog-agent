// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build docker

package listeners

import (
	"sync"
	"time"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/ecs"
)

// ECSListener implements the ServiceListener interface for fargate-backed ECS cluster.
// It pulls its tasks container list periodically and checks for
// new containers to monitor, and old containers to stop monitoring
type ECSListener struct {
	task         ecs.TaskMetadata
	services     map[string]Service // maps container IDs to services
	newService   chan<- Service
	delService   chan<- Service
	stop         chan bool
	t            *time.Ticker
	healthTicker *time.Ticker
	healthToken  health.ID
	m            sync.RWMutex
}

// ECSService implements and store results from the Service interface for the ECS listener
type ECSService struct {
	ID            ID
	ADIdentifiers []string
	Hosts         map[string]string
	Ports         []int
	Pid           int
	Tags          []string
	clusterName   string
	taskFamily    string
	taskVersion   string
}

func init() {
	Register("ecs", NewECSListener)
}

// NewECSListener creates an ECSListener
func NewECSListener() (ServiceListener, error) {
	return &ECSListener{
		services:     make(map[string]Service),
		stop:         make(chan bool),
		t:            time.NewTicker(2 * time.Second),
		healthTicker: time.NewTicker(15 * time.Second),
		healthToken:  health.Register("ad-ecslistener"),
	}, nil
}

// Listen polls regularly container-related events from the ECS task metadata endpoint and report said containers as Services.
func (l *ECSListener) Listen(newSvc chan<- Service, delSvc chan<- Service) {
	// setup the I/O channels
	l.newService = newSvc
	l.delService = delSvc

	go func() {
		for {
			select {
			case <-l.stop:
				l.healthTicker.Stop()
				health.Deregister(l.healthToken)
				return
			case <-l.healthTicker.C:
				health.Ping(l.healthToken)
			case <-l.t.C:
				l.refreshServices()
			}
		}
	}()
}

// Stop queues a shutdown of ECSListener
func (l *ECSListener) Stop() {
	l.stop <- true
}

// refreshServices queries the task metadata endpoint for fresh info
// compares the container list to the local cache and sends new/dead services
// over newService and delService accordingly
func (l *ECSListener) refreshServices() {
	meta, err := ecs.GetTaskMetadata()
	if err != nil {
		log.Errorf("failed to get task metadata, not refreshing services - %s", err)
		return
	} else if meta.KnownStatus != "RUNNING" {
		log.Debugf("task %s is not in RUNNING state yet, not refreshing services", meta.Family)
		return
	}
	l.task = meta

	// if not found and running, add it. Else no-op
	// at the end, compare what we saw and what is cached and kill what's not there anymore
	notSeen := make(map[string]interface{})
	for i := range l.services {
		notSeen[i] = nil
	}

	for _, c := range meta.Containers {
		if _, found := l.services[c.DockerID]; found {
			delete(notSeen, c.DockerID)
			continue
		}
		if c.KnownStatus != "RUNNING" {
			log.Debugf("container %s is in status %s - skipping", c.DockerID, c.KnownStatus)
			continue
		}
		s, err := l.createService(c)
		if err != nil {
			log.Errorf("couldn't create a service out of container %s - Auto Discovery will ignore it", c.DockerID)
			continue
		}
		l.m.Lock()
		l.services[c.DockerID] = &s
		l.m.Unlock()
		l.newService <- &s
		delete(notSeen, c.DockerID)
	}

	for cID := range notSeen {
		l.m.RLock()
		l.delService <- l.services[cID]
		l.m.RUnlock()
		l.m.Lock()
		delete(l.services, cID)
		l.m.Unlock()
	}
}

func (l *ECSListener) createService(c ecs.Container) (ECSService, error) {
	cID := ID(c.DockerID)
	svc := ECSService{
		ID:          cID,
		clusterName: l.task.ClusterName,
		taskFamily:  l.task.Family,
		taskVersion: l.task.Version,
	}

	// ADIdentifiers
	image := c.Image
	labels := c.Labels
	svc.ADIdentifiers = ComputeContainerServiceIDs(c.DockerID, image, labels)

	// Host
	ips := make(map[string]string)

	for _, net := range c.Networks {
		if net.NetworkMode == "awsvpc" && len(net.IPv4Addresses) > 0 {
			ips["awsvpc"] = string(net.IPv4Addresses[0])
		}
	}
	svc.Hosts = ips

	// Tags
	entity := docker.ContainerIDToEntityName(string(c.DockerID))
	tags, err := tagger.Tag(entity, false)
	if err != nil {
		log.Errorf("Failed to extract tags for container %s - %s", cID[:12], err)
	}
	svc.Tags = tags

	// Ports and Pid
	svc.Ports = nil
	svc.Pid = -1

	return svc, err
}

// GetID returns the service ID
func (s *ECSService) GetID() ID {
	return s.ID
}

// GetADIdentifiers returns a set of AD identifiers for a container.
// These id are sorted to reflect the priority we want the ConfigResolver to
// use when matching a template.
//
// When the special identifier label in `identifierLabel` is set by the user,
// it overrides any other meaning of template identification for the service
// and the return value will contain only the label value.
//
// If the special label was not set, the priority order is the following:
//   1. Long image name
//   2. Short image name
func (s *ECSService) GetADIdentifiers() ([]string, error) {
	return s.ADIdentifiers, nil
}

// GetHosts returns the container's hosts
// TODO: using localhost should usually be enough
func (s *ECSService) GetHosts() (map[string]string, error) {
	return s.Hosts, nil
}

// GetPorts returns nil and an error because port is not supported in Fargate-based ECS
func (s *ECSService) GetPorts() ([]int, error) {
	return nil, ErrNotSupported
}

// GetTags retrieves a container's tags
func (s *ECSService) GetTags() ([]string, error) {
	return s.Tags, nil
}

// GetPid inspect the container and return its pid
// TODO: not supported as pid is not in the metadata api
func (s *ECSService) GetPid() (int, error) {
	return -1, ErrNotSupported
}
