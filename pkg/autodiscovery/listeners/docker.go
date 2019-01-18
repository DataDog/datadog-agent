// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build docker

package listeners

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"

	"github.com/docker/docker/api/types"
	"github.com/docker/go-connections/nat"

	"github.com/DataDog/datadog-agent/cmd/agent/common/signals"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// DockerListener implements the ServiceListener interface.
// It listens for Docker events and reports container updates to Auto Discovery
// It also holds a cache of services that the AutoConfig can query to
// match templates against.
type DockerListener struct {
	dockerUtil *docker.DockerUtil
	filter     *containers.Filter
	services   map[string]Service
	newService chan<- Service
	delService chan<- Service
	stop       chan bool
	health     *health.Handle
	m          sync.RWMutex
}

// DockerService implements and store results from the Service interface for the Docker listener
type DockerService struct {
	sync.RWMutex
	cID           string
	adIdentifiers []string
	hosts         map[string]string
	ports         []ContainerPort
	pid           int
	hostname      string
	creationTime  integration.CreationTime
}

func init() {
	Register("docker", NewDockerListener)
}

// NewDockerListener creates a client connection to Docker and instantiate a DockerListener with it
func NewDockerListener() (ServiceListener, error) {
	d, err := docker.GetDockerUtil()
	if err != nil {
		return nil, err
	}
	filter, err := containers.NewFilterFromConfigIncludePause()
	if err != nil {
		return nil, err
	}
	return &DockerListener{
		dockerUtil: d,
		filter:     filter,
		services:   make(map[string]Service),
		stop:       make(chan bool),
		health:     health.Register("ad-dockerlistener"),
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
				l.health.Deregister()
				return
			case <-l.health.C:
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
// creates services for them, and pass them to the AutoConfig.
// It is typically called at start up.
func (l *DockerListener) init() {
	l.m.Lock()
	defer l.m.Unlock()

	containers, err := l.dockerUtil.RawContainerList(types.ContainerListOptions{})
	if err != nil {
		log.Errorf("Couldn't retrieve container list - %s", err)
	}

	for _, co := range containers {
		if l.isExcluded(co) {
			continue // helper method already logs
		}
		var svc Service

		if findKubernetesInLabels(co.Labels) {
			svc = &DockerKubeletService{
				DockerService: DockerService{
					cID:           co.ID,
					adIdentifiers: l.getConfigIDFromPs(co),
					// Host and Ports will be looked up when needed
				},
			}
		} else {
			svc = &DockerService{
				cID:           co.ID,
				adIdentifiers: l.getConfigIDFromPs(co),
				hosts:         l.getHostsFromPs(co),
				ports:         l.getPortsFromPs(co),
				creationTime:  integration.Before,
			}
		}
		l.newService <- svc
		l.services[co.ID] = svc
	}
}

// processEvent takes a ContainerEvent, tries to find a service linked to it, and
// figure out if the AutoConfig could be interested to inspect it.
func (l *DockerListener) processEvent(e *docker.ContainerEvent) {
	cID := e.ContainerID

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
// and tells the AutoConfig that this service started.
func (l *DockerListener) createService(cID string) {
	var svc Service

	// Detect whether that container is managed by Kubernetes
	var isKube bool
	cInspect, err := l.dockerUtil.Inspect(cID, false)
	if err != nil {
		log.Errorf("Failed to inspect container %s - %s", cID[:12], err)
	} else {
		image, err := l.dockerUtil.ResolveImageName(cInspect.Image)
		if err != nil {
			log.Warnf("error while resolving image name: %s", err)
			image = ""
		}
		if l.filter.IsExcluded(cInspect.Name, image) {
			log.Debugf("container %s filtered out: name %q image %q", cID[:12], cInspect.Name, image)
			return
		}
		if findKubernetesInLabels(cInspect.Config.Labels) {
			isKube = true
		}
	}

	if isKube {
		svc = &DockerKubeletService{
			DockerService: DockerService{
				cID: cID,
			},
		}
	} else {
		svc = &DockerService{
			cID:          cID,
			creationTime: integration.After,
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
	l.services[cID] = svc
	l.m.Unlock()

	l.newService <- svc
}

// removeService takes a container ID, removes the related service from its cache
// and tells the AutoConfig that this service stopped.
func (l *DockerListener) removeService(cID string) {
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
	entity := docker.ContainerIDToEntityName(co.ID)
	return ComputeContainerServiceIDs(entity, image, co.Labels)
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

	if len(ips) == 0 {
		// More cases require a container inspect, delay it until
		// template resolution, when GetHosts will be called.
		return nil
	}
	return ips
}

// getPortsFromPs gets the service ports of a container.
func (l *DockerListener) getPortsFromPs(co types.Container) []ContainerPort {
	// Nil array by default, we'll need to inspect the container
	// later if we don't find any port in the PS
	var ports []ContainerPort

	for _, p := range co.Ports {
		ports = append(ports, ContainerPort{int(p.PrivatePort), ""})
	}
	sort.Slice(ports, func(i, j int) bool {
		return ports[i].Port < ports[j].Port
	})
	return ports
}

// GetEntity returns the unique entity name linked to that service
func (s *DockerService) GetEntity() string {
	return docker.ContainerIDToEntityName(s.cID)
}

func (l *DockerListener) isExcluded(co types.Container) bool {
	image, err := l.dockerUtil.ResolveImageName(co.Image)
	if err != nil {
		log.Warnf("error while resolving image name: %s", err)
		image = ""
	}
	for _, name := range co.Names {
		if l.filter.IsExcluded(name, image) {
			log.Debugf("container %s filtered out: name %q image %q", co.ID[:12], name, image)
			return true
		}
	}
	return false
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
	if len(s.adIdentifiers) == 0 {
		du, err := docker.GetDockerUtil()
		if err != nil {
			return []string{}, err
		}
		cj, err := du.Inspect(s.cID, false)
		if err != nil {
			return []string{}, err
		}
		image, err := du.ResolveImageName(cj.Image)
		if err != nil {
			log.Warnf("error while resolving image name: %s", err)
		}
		entity := docker.ContainerIDToEntityName(s.cID)
		s.adIdentifiers = ComputeContainerServiceIDs(entity, image, cj.Config.Labels)
	}

	return s.adIdentifiers, nil
}

// GetHosts returns the container's hosts
func (s *DockerService) GetHosts() (map[string]string, error) {
	s.Lock()
	defer s.Unlock()

	if s.hosts != nil {
		return s.hosts, nil
	}

	ips := make(map[string]string)
	du, err := docker.GetDockerUtil()
	if err != nil {
		return nil, err
	}
	cInspect, err := du.Inspect(s.cID, false)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect container %s", s.cID[:12])
	}
	if cInspect.NetworkSettings != nil {
		for net, settings := range cInspect.NetworkSettings.Networks {
			if len(settings.IPAddress) > 0 {
				ips[net] = settings.IPAddress
			}
		}
	}

	rancherIP, found := docker.FindRancherIPInLabels(cInspect.Config.Labels)
	if found {
		ips["rancher"] = rancherIP
	}

	// Some CNI solutions (including ECS awsvpc) do not assign an IP
	// through docker, but set a valid reachable hostname. Use it if
	// no IP is discovered.
	if len(ips) == 0 && cInspect.Config != nil && len(cInspect.Config.Hostname) > 0 {
		ips["hostname"] = cInspect.Config.Hostname
	}

	s.hosts = ips
	return ips, nil
}

// GetPorts returns the container's ports
func (s *DockerService) GetPorts() ([]ContainerPort, error) {
	if s.ports != nil {
		return s.ports, nil
	}

	// Make a non-nil array to avoid re-running if we find zero port
	ports := []ContainerPort{}

	du, err := docker.GetDockerUtil()
	if err != nil {
		return ports, err
	}
	cInspect, err := du.Inspect(s.cID, false)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect container %s", s.cID[:12])
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
		log.Infof("using ExposedPorts for container %s as no port bindings are listed", s.cID[:12])
		for p := range cInspect.Config.ExposedPorts {
			out, err := parseDockerPort(p)
			if err != nil {
				log.Warn(err.Error())
				continue
			}
			ports = append(ports, out...)
		}
	}

	sort.Slice(ports, func(i, j int) bool {
		return ports[i].Port < ports[j].Port
	})
	s.ports = ports
	return ports, nil
}

func parseDockerPort(port nat.Port) ([]ContainerPort, error) {
	var output []ContainerPort

	// Try to parse a port range, eg. 22-25
	first, last, err := port.Range()
	if err == nil && last > first {
		for p := first; p <= last; p++ {
			output = append(output, ContainerPort{p, ""})
		}
		return output, nil
	}

	// Try to parse a single port (most common case)
	p := port.Int()
	if p > 0 {
		output = append(output, ContainerPort{p, ""})
		return output, nil
	}

	return output, fmt.Errorf("failed to extract port from: %v", port)
}

// GetTags retrieves tags using the Tagger
func (s *DockerService) GetTags() ([]string, error) {
	tags, err := tagger.Tag(s.GetEntity(), tagger.ChecksCardinality)
	if err != nil {
		return []string{}, err
	}

	return tags, nil
}

// GetPid inspect the container an return its pid
func (s *DockerService) GetPid() (int, error) {
	// Try to inspect container to get the pid if not defined
	if s.pid <= 0 {
		du, err := docker.GetDockerUtil()
		if err != nil {
			return -1, err
		}
		cj, err := du.Inspect(s.cID, false)
		if err != nil {
			return -1, err
		}
		s.pid = cj.State.Pid
	}

	return s.pid, nil
}

// GetHostname returns hostname.domainname for the container
func (s *DockerService) GetHostname() (string, error) {
	if s.hostname != "" {
		return s.hostname, nil
	}

	du, err := docker.GetDockerUtil()
	if err != nil {
		return "", err
	}
	cInspect, err := du.Inspect(s.cID, false)
	if err != nil {
		return "", fmt.Errorf("failed to inspect container %s", s.cID[:12])
	}
	if cInspect.Config == nil {
		return "", fmt.Errorf("invalid inspect for container %s", s.cID[:12])
	}
	if cInspect.Config.Hostname == "" {
		return "", fmt.Errorf("empty hostname for container %s", s.cID[:12])
	}

	s.hostname = cInspect.Config.Hostname
	return s.hostname, nil
}

// GetCreationTime returns the creation time of the container compare to the agent start.
func (s *DockerService) GetCreationTime() integration.CreationTime {
	return s.creationTime
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
