// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package listeners

import (
	"context"
	"fmt"
	"io"
	"time"

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
	Services   map[ID]Service
	newService chan Service
	delService chan Service
	stop       chan bool
}

// NewDockerListener creates a client connection to Docker and instanciate a DockerListener with it
// TODO: TLS support
func NewDockerListener(newSvc, delSvc chan Service) (*DockerListener, error) {
	c, err := client.NewEnvClient()
	if err != nil {
		return nil, fmt.Errorf("Failed to connect to Docker, auto discovery will not work: %s", err)
	}

	svc := make(map[ID]Service)
	stop := make(chan bool)

	return &DockerListener{c, svc, newSvc, delSvc, stop}, nil
}

// Listen streams container-related events from Docker and report said containers as Services.
func (l *DockerListener) Listen() {
	// Init
	l.Services = l.GetCurrentServices()

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
					panic(err)
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

// GetCurrentServices looks at currently running Docker containers,
// creates services for them, and pass them to the ConfigResolver.
// It is typically called at start up.
func (l *DockerListener) GetCurrentServices() map[ID]Service {
	containers, err := l.Client.ContainerList(context.Background(), types.ContainerListOptions{})
	if err != nil {
		log.Errorf("Couldn't retrieve container list - %s", err)
	}

	services := make(map[ID]Service, 0)
	for _, co := range containers {
		id := ID(co.ID)
		ADidentifiers := l.getConfigIDFromPs(co)
		hosts := l.getHostsFromPs(co)
		ports := l.getPortsFromPs(co)
		tags := l.getTagsFromPs(co)
		svc := Service{id, ADidentifiers, hosts, ports, tags}
		l.newService <- svc
		services[id] = svc
	}
	return services
}

// processEvent takes a Docker Message, tries to find a service linked to it, and
// figure out if the ConfigResolver could be interested to inspect it.
func (l *DockerListener) processEvent(e events.Message) {
	cID := ID(e.Actor.ID)
	if _, found := l.Services[cID]; found {
		if e.Action == "die" {
			l.removeService(cID)
		} else {
			panic("TODO - this shouldn't happen")
		}
	} else {
		if e.Action == "start" {
			l.createService(cID)
		} else {
			panic("TODO - this shouldn't happen")
		}
	}
}

// startService takes a container ID, create a service for it in its cache
// and tells the ConfigResolver that this service started.
func (l *DockerListener) createService(cID ID) {
	co, err := l.Client.ContainerInspect(context.Background(), string(cID))
	if err != nil {
		log.Errorf("Failed to inspect container %s - %s", cID[:12], err)

	}
	svc := Service{
		cID,
		l.getConfigIDFromInspect(co),
		l.getHostsFromInspect(co),
		l.getPortsFromInspect(co),
		l.getTagsFromInspect(co),
	}
	l.Services[ID(cID)] = svc
	l.newService <- svc
}

// removeService takes a container ID, removes the related service from its cache
// and tells the ConfigResolver that this service stopped.
func (l *DockerListener) removeService(cID ID) {
	if svc, ok := l.Services[cID]; ok {
		delete(l.Services, cID)
		l.delService <- svc
	} else {
		log.Debugf("Container %s not found, not removing", cID[:12])
	}

}

// getConfigIDFromInspect returns a set of AD identifiers for a container.
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
func (l *DockerListener) getConfigIDFromInspect(co types.ContainerJSON) []string {
	// check for an identifier label
	for l, v := range co.Config.Labels {
		if l == identifierLabel {
			return []string{v}
		}
	}

	ids := []string{}

	// use the image name
	ids = append(ids, co.Image) // TODO: check if it's the sha256
	// TODO: add the short name with lower priority

	return ids
}

// getHosts gets the addresss (for now IP address only) of a container on all its networks.
// TODO: use the k8s API when no network config is found
func (l *DockerListener) getHostsFromInspect(co types.ContainerJSON) map[string]string {
	ips := make(map[string]string)
	if co.NetworkSettings != nil {
		for net, settings := range co.NetworkSettings.Networks {
			ips[net] = settings.IPAddress
		}
	}
	return ips
}

// getPorts gets the service ports of a container.
// TODO: use the k8s API
func (l *DockerListener) getPortsFromInspect(co types.ContainerJSON) []int {
	ports := make([]int, 0)

	for p := range co.NetworkSettings.Ports {
		ports = append(ports, p.Int())
	}
	return ports
}

// getTags gets tags for a container.
// TODO: use the container ID only and rely on container metadata providers?
func (l *DockerListener) getTagsFromInspect(co types.ContainerJSON) []string {
	return []string{}
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
	// check for an identifier label
	for l, v := range co.Labels {
		if l == identifierLabel {
			return []string{v}
		}
	}

	ids := []string{}

	// use the image name
	ids = append(ids, co.Image) // TODO: check if it's the sha256
	// TODO: add the short name with lower priority

	return ids
}

// getHosts gets the addresss (for now IP address only) of a container on all its networks.
// TODO: use the k8s API when no network config is found
func (l *DockerListener) getHostsFromPs(co types.Container) map[string]string {
	ips := make(map[string]string)
	if co.NetworkSettings != nil {
		for net, settings := range co.NetworkSettings.Networks {
			ips[net] = settings.IPAddress
		}
	}
	return ips
}

// getPorts gets the service ports of a container.
// TODO: use the k8s API
func (l *DockerListener) getPortsFromPs(co types.Container) []int {
	ports := make([]int, 0)

	for _, p := range co.Ports {
		ports = append(ports, int(p.PrivatePort))
	}
	return ports
}

// getTags gets tags for a container.
// TODO: use the container ID only and rely on container metadata providers?
func (l *DockerListener) getTagsFromPs(co types.Container) []string {
	return []string{}
}
