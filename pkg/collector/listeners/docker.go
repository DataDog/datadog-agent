// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build docker

package listeners

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/cmd/agent/common/signals"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	log "github.com/cihub/seelog"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
)

const (
	identifierLabel string = "io.datadog.check.id"
)

// DockerListener implements the ServiceListener interface.
// It listens for Docker events and reports container updates to Auto Discovery
// It also holds a cache of services that the ConfigResolver can query to
// match templates against.
type DockerListener struct {
	Client     *client.Client
	dockerUtil *docker.DockerUtil
	services   map[ID]Service
	newService chan<- Service
	delService chan<- Service
	stop       chan bool
	m          sync.RWMutex
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
	c, err := docker.ConnectToDocker()
	if err != nil {
		return nil, fmt.Errorf("Failed to connect to Docker, auto discovery will not work: %s", err)
	}
	d, err := docker.GetDockerUtil()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Docker, auto discovery will not work: %s", err)
	}
	return &DockerListener{
		Client:     c,
		dockerUtil: d,
		services:   make(map[ID]Service),
		stop:       make(chan bool),
	}, nil
}

// Listen streams container-related events from Docker and report said containers as Services.
func (l *DockerListener) Listen(newSvc chan<- Service, delSvc chan<- Service) {
	// setup the I/O channels
	l.newService = newSvc
	l.delService = delSvc

	// process containers that might be already running
	l.init()

	// filters only match start/stop container events
	filters := filters.NewArgs()
	filters.Add("type", "container")
	filters.Add("event", "start")
	filters.Add("event", "die")
	eventOptions := types.EventsOptions{
		Since:   fmt.Sprintf("%d", time.Now().Unix()),
		Filters: filters,
	}

	messages, errs := l.Client.Events(context.Background(), eventOptions)

	go func() {
		for {
			select {
			case <-l.stop:
				return
			case msg := <-messages:
				l.processEvent(msg)
			case err := <-errs:
				if err != nil && err != io.EOF {
					log.Errorf("Docker listener error: %v", err)
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

	containers, err := l.Client.ContainerList(context.Background(), types.ContainerListOptions{})
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

// processEvent takes a Docker Message, tries to find a service linked to it, and
// figure out if the ConfigResolver could be interested to inspect it.
func (l *DockerListener) processEvent(e events.Message) {
	cID := ID(e.Actor.ID)

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
	return computeDockerIDs(co.ID, image, co.Labels)
}

// getHostsFromPs gets the addresss (for now IP address only) of a container on all its networks.
func (l *DockerListener) getHostsFromPs(co types.Container) map[string]string {
	ips := make(map[string]string)
	if co.NetworkSettings != nil {
		for net, settings := range co.NetworkSettings.Networks {
			ips[net] = settings.IPAddress
		}
	}
	return ips
}

// getPortsFromPs gets the service ports of a container.
func (l *DockerListener) getPortsFromPs(co types.Container) []int {
	ports := make([]int, 0)

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
		s.ADIdentifiers = computeDockerIDs(string(s.ID), image, cj.Config.Labels)
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
		return ips, err
	}
	cInspect, err := du.Inspect(string(s.ID), false)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect container %s", string(s.ID)[:12])
	}
	for net, settings := range cInspect.NetworkSettings.Networks {
		ips[net] = settings.IPAddress
	}

	s.Hosts = ips
	return ips, nil
}

// GetPorts returns the container's ports
func (s *DockerService) GetPorts() ([]int, error) {
	if s.Ports != nil {
		return s.Ports, nil
	}

	ports := make([]int, 0)
	du, err := docker.GetDockerUtil()
	if err != nil {
		return ports, err
	}
	cInspect, err := du.Inspect(string(s.ID), false)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect container %s", string(s.ID)[:12])
	}

	for p := range cInspect.NetworkSettings.Ports {
		portStr := string(p)
		if strings.Contains(portStr, "/") {
			portStr = strings.Split(portStr, "/")[0]
		}
		port, err := strconv.Atoi(portStr)
		if err != nil {
			log.Warnf("failed to extract port %s", string(p))
		}
		ports = append(ports, port)
	}
	sort.Ints(ports)
	s.Ports = ports
	return ports, nil
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

// computeDockerIDs factors in code for getConfigIDFromPs and GetADIdentifiers
// it assumes the image name's sha is already resolved via docker.ResolveImageName
func computeDockerIDs(cid string, image string, labels map[string]string) []string {
	ids := []string{}

	// check for an identifier label
	for l, v := range labels {
		if l == identifierLabel {
			ids = append(ids, v)
			// Let's not return the image name if we find the label
			return ids
		}
	}

	// add the container ID for templates in labels/annotations
	ids = append(ids, docker.ContainerIDToEntityName(cid))

	// add the image names (long then short if different)
	long, short, _, err := docker.SplitImageName(image)
	if err != nil {
		log.Warnf("error while spliting image name: %s", err)
	}
	if len(long) > 0 {
		ids = append(ids, long)
	}
	if len(short) > 0 && short != long {
		ids = append(ids, short)
	}

	return ids
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
