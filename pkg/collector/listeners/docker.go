// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build docker

package listeners

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	log "github.com/cihub/seelog"
	"github.com/docker/docker/api/types"
	"github.com/docker/go-connections/nat"

	"github.com/DataDog/datadog-agent/cmd/agent/common/signals"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
)

// DockerListener implements the ServiceListener interface.
// It listens for Docker events and reports container updates to Auto Discovery
// It also holds a cache of services that the ConfigResolver can query to
// match templates against.
type DockerListener struct {
	dockerUtil   *docker.DockerUtil
	services     map[ID]Service
	newService   chan<- Service
	delService   chan<- Service
	stop         chan bool
	healthTicker *time.Ticker
	healthToken  health.ID
	m            sync.RWMutex
}

// DockerService implements and store results from the Service interface for the Docker listener
type DockerService struct {
	ID            ID
	ADIdentifiers []string
	Hosts         map[string]string
	Ports         []int
	Pid           int
}

func init() {
	Register("docker", NewDockerListener)
}

// NewDockerListener creates a client connection to Docker and instantiate a DockerListener with it
// TODO: TLS support
func NewDockerListener() (ServiceListener, error) {
	d, err := docker.GetDockerUtil()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Docker, auto discovery will not work: %s", err)
	}
	return &DockerListener{
		dockerUtil:   d,
		services:     make(map[ID]Service),
		stop:         make(chan bool),
		healthTicker: time.NewTicker(15 * time.Second),
		healthToken:  health.Register("ad-dockerlistener"),
	}, nil
}

// Listen streams container-related events from Docker and report said containers as Services.
func (l *DockerListener) Listen(newSvc chan<- Service, delSvc chan<- Service) {
	// setup the I/O channels
	l.newService = newSvc
	l.delService = delSvc

	// process containers that might be already running
	l.init()

	messages, errs, err := l.dockerUtil.SubscribeToContainerEvents("DockerListener")
	if err != nil {
		log.Errorf("can't listen to docker events: %v", err)
		signals.ErrorStopper <- true
		return
	}

	go func() {
		for {
			select {
			case <-l.stop:
				l.dockerUtil.UnsubscribeFromContainerEvents("DockerListener")
				l.healthTicker.Stop()
				health.Deregister(l.healthToken)
				return
			case <-l.healthTicker.C:
				health.Ping(l.healthToken)
			case msg := <-messages:
				l.processEvent(msg)
			case err := <-errs:
				if err != nil && err != io.EOF {
					log.Errorf("docker listener error: %v", err)
					signals.ErrorStopper <- true
				}
				return
			}
		}
	}()
}

// Stop queues a shutdown of DockerListener
func (l *DockerListener) Stop() {
	l.stop <- true
}

// init looks at currently running Docker containers,
// creates services for them, and pass them to the ConfigResolver.
// It is typically called at start up.
func (l *DockerListener) init() {
	l.m.Lock()
	defer l.m.Unlock()

	containers, err := l.dockerUtil.ContainerList(context.Background(), types.ContainerListOptions{})
	if err != nil {
		log.Errorf("Couldn't retrieve container list - %s", err)
	}

	for _, co := range containers {
		id := ID(co.ID)
		var svc Service

		if findKubernetesInLabels(co.Labels) {
			svc = &DockerKubeletService{
				DockerService: DockerService{
					ID:            id,
					ADIdentifiers: l.getConfigIDFromPs(co),
					// Host and Ports will be looked up when needed
				},
			}
		} else {
			svc = &DockerService{
				ID:            id,
				ADIdentifiers: l.getConfigIDFromPs(co),
				Hosts:         l.getHostsFromPs(co),
				Ports:         l.getPortsFromPs(co),
			}
		}
		l.newService <- svc
		l.services[id] = svc
	}
}

// GetServices returns a copy of the current services
func (l *DockerListener) GetServices() map[ID]Service {
	l.m.RLock()
	defer l.m.RUnlock()

	ret := make(map[ID]Service)
	for k, v := range l.services {
		ret[k] = v
	}

	return ret
}

// processEvent takes a ContainerEvent, tries to find a service linked to it, and
// figure out if the ConfigResolver could be interested to inspect it.
func (l *DockerListener) processEvent(e *docker.ContainerEvent) {
	cID := ID(e.ContainerID)

	l.m.RLock()
	_, found := l.services[cID]
	l.m.RUnlock()

	if found {
		if e.Action == "die" {
			l.removeService(cID)
		} else {
			// FIXME sometimes the agent's container's events are picked up twice at startup
			log.Debugf("Expected die for container %s got %s: skipping event", cID[:12], e.Action)
			return
		}
	} else {
		// we might receive a `die` event for an unrelated container we don't
		// care about, let's ignore it.
		if e.Action == "start" {
			l.createService(cID)
		}
	}
}

// createService takes a container ID, create a service for it in its cache
// and tells the ConfigResolver that this service started.
func (l *DockerListener) createService(cID ID) {
	var svc Service

	// Detect whether that container is managed by Kubernetes
	cInspect, err := l.dockerUtil.Inspect(string(cID), false)
	if err != nil {
		log.Errorf("Failed to inspect container %s - %s", cID[:12], err)
	}
	if findKubernetesInLabels(cInspect.Config.Labels) {
		svc = &DockerKubeletService{
			DockerService: DockerService{
				ID: cID,
			},
		}
	} else {
		svc = &DockerService{
			ID: cID,
		}
	}

	_, err = svc.GetADIdentifiers()
	if err != nil {
		log.Errorf("Failed to inspect container %s - %s", cID[:12], err)
	}
	_, err = svc.GetHosts()
	if err != nil {
		log.Errorf("Failed to inspect container %s - %s", cID[:12], err)
	}
	_, err = svc.GetPorts()
	if err != nil {
		log.Errorf("Failed to inspect container %s - %s", cID[:12], err)
	}
	_, err = svc.GetPid()
	if err != nil {
		log.Errorf("Failed to inspect container %s - %s", cID[:12], err)
	}
	_, err = svc.GetTags()
	if err != nil {
		log.Errorf("Failed to inspect container %s - %s", cID[:12], err)
	}

	l.m.Lock()
	l.services[ID(cID)] = svc
	l.m.Unlock()

	l.newService <- svc
}

// removeService takes a container ID, removes the related service from its cache
// and tells the ConfigResolver that this service stopped.
func (l *DockerListener) removeService(cID ID) {
	l.m.RLock()
	svc, ok := l.services[cID]
	l.m.RUnlock()

	if ok {
		l.m.Lock()
		delete(l.services, cID)
		l.m.Unlock()

		l.delService <- svc
	} else {
		log.Debugf("Container %s not found, not removing", cID[:12])
	}
}

// getConfigIDFromPs returns a set of AD identifiers for a container.
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
func (l *DockerListener) getConfigIDFromPs(co types.Container) []string {
	image, err := l.dockerUtil.ResolveImageName(co.Image)
	if err != nil {
		log.Warnf("error while resolving image name: %s", err)
	}
	return ComputeContainerServiceIDs(co.ID, image, co.Labels)
}

// getHostsFromPs gets the addresss (for now IP address only) of a container on all its networks.
func (l *DockerListener) getHostsFromPs(co types.Container) map[string]string {
	ips := make(map[string]string)
	if co.NetworkSettings != nil {
		for net, settings := range co.NetworkSettings.Networks {
			if len(settings.IPAddress) > 0 {
				ips[net] = settings.IPAddress
			}
		}
	}

	rancherIP, found := docker.FindRancherIPInLabels(co.Labels)
	if found {
		ips["rancher"] = rancherIP
	}
	return ips
}

// getPortsFromPs gets the service ports of a container.
func (l *DockerListener) getPortsFromPs(co types.Container) []int {
	// Nil array by default, we'll need to inspect the container
	// later if we don't find any port in the PS
	var ports []int

	for _, p := range co.Ports {
		ports = append(ports, int(p.PrivatePort))
	}
	return ports
}

// GetID returns the service ID
func (s *DockerService) GetID() ID {
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
func (s *DockerService) GetADIdentifiers() ([]string, error) {
	if len(s.ADIdentifiers) == 0 {
		du, err := docker.GetDockerUtil()
		if err != nil {
			return []string{}, err
		}
		cj, err := du.Inspect(string(s.ID), false)
		if err != nil {
			return []string{}, err
		}
		image, err := du.ResolveImageName(cj.Image)
		if err != nil {
			log.Warnf("error while resolving image name: %s", err)
		}
		s.ADIdentifiers = ComputeContainerServiceIDs(string(s.ID), image, cj.Config.Labels)
	}

	return s.ADIdentifiers, nil
}

// GetHosts returns the container's hosts
func (s *DockerService) GetHosts() (map[string]string, error) {
	if s.Hosts != nil {
		return s.Hosts, nil
	}

	ips := make(map[string]string)
	du, err := docker.GetDockerUtil()
	if err != nil {
		return nil, err
	}
	cInspect, err := du.Inspect(string(s.ID), false)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect container %s", string(s.ID)[:12])
	}
	for net, settings := range cInspect.NetworkSettings.Networks {
		if len(settings.IPAddress) > 0 {
			ips[net] = settings.IPAddress
		}
	}

	rancherIP, found := docker.FindRancherIPInLabels(cInspect.Config.Labels)
	if found {
		ips["rancher"] = rancherIP
	}

	s.Hosts = ips
	return ips, nil
}

// GetPorts returns the container's ports
func (s *DockerService) GetPorts() ([]int, error) {
	if s.Ports != nil {
		return s.Ports, nil
	}

	// Make a non-nil array to avoid re-running if we find zero port
	ports := []int{}

	du, err := docker.GetDockerUtil()
	if err != nil {
		return ports, err
	}
	cInspect, err := du.Inspect(string(s.ID), false)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect container %s", string(s.ID)[:12])
	}

	switch {
	case cInspect.NetworkSettings != nil && len(cInspect.NetworkSettings.Ports) > 0:
		for p := range cInspect.NetworkSettings.Ports {
			out, err := parseDockerPort(p)
			if err != nil {
				log.Warn(err.Error())
				continue
			}
			ports = append(ports, out...)
		}
	case cInspect.Config != nil && len(cInspect.Config.ExposedPorts) > 0:
		log.Infof("using ExposedPorts for container %s as no port bindings are listed", string(s.ID)[:12])
		for p := range cInspect.Config.ExposedPorts {
			out, err := parseDockerPort(p)
			if err != nil {
				log.Warn(err.Error())
				continue
			}
			ports = append(ports, out...)
		}
	}

	sort.Ints(ports)
	s.Ports = ports
	return ports, nil
}

func parseDockerPort(port nat.Port) ([]int, error) {
	var output []int

	// Try to parse a port range, eg. 22-25
	first, last, err := port.Range()
	if err == nil && last > first {
		for p := first; p <= last; p++ {
			output = append(output, p)
		}
		return output, nil
	}

	// Try to parse a single port (most common case)
	p := port.Int()
	if p > 0 {
		output = append(output, p)
		return output, nil
	}

	return output, fmt.Errorf("failed to extract port from: %v", port)
}

// GetTags retrieves tags using the Tagger
func (s *DockerService) GetTags() ([]string, error) {
	entity := docker.ContainerIDToEntityName(string(s.ID))
	tags, err := tagger.Tag(entity, false)
	if err != nil {
		return []string{}, err
	}

	return tags, nil
}

// GetPid inspect the container an return its pid
func (s *DockerService) GetPid() (int, error) {
	// Try to inspect container to get the pid if not defined
	if s.Pid <= 0 {
		du, err := docker.GetDockerUtil()
		if err != nil {
			return -1, err
		}
		cj, err := du.Inspect(string(s.ID), false)
		if err != nil {
			return -1, err
		}
		s.Pid = cj.State.Pid
	}

	return s.Pid, nil
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
