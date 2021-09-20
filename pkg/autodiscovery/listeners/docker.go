// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build !serverless

package listeners

import (
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

func init() {
	Register("docker", NewDockerListener)
}

// DockerListener listens to container creation through a subscription to the
// workloadmeta store.
type DockerListener struct {
	store *workloadmeta.Store
	stop  chan struct{}

	mu       sync.RWMutex
	filters  *containerFilters
	services map[string]Service

	newService chan<- Service
	delService chan<- Service
}

// NewDockerListener returns a new DockerListener.
func NewDockerListener() (ServiceListener, error) {
	filters, err := newContainerFilters()
	if err != nil {
		return nil, err
	}

	return &DockerListener{
		store:    workloadmeta.GetGlobalStore(),
		filters:  filters,
		services: make(map[string]Service),
		stop:     make(chan struct{}),
	}, nil
}

// Listen starts listening to events from the workloadmeta store.
func (l *DockerListener) Listen(newSvc chan<- Service, delSvc chan<- Service) {
	l.newService = newSvc
	l.delService = delSvc

	const name = "ad-dockerlistener"

	ch := l.store.Subscribe(name, workloadmeta.NewFilter(
		[]workloadmeta.Kind{workloadmeta.KindContainer},
		[]string{"docker"},
	))
	health := health.RegisterLiveness(name)

	log.Info("docker listener initialized successfully")

	go func() {
		for {
			select {
			case evBundle := <-ch:
				l.processEvents(evBundle)

			case <-health.C:

			case <-l.stop:
				err := health.Deregister()
				if err != nil {
					log.Warnf("error de-registering health check: %s", err)
				}

				l.store.Unsubscribe(ch)

				return
			}
		}
	}()
}

func (l *DockerListener) processEvents(evBundle workloadmeta.EventBundle) {
	// close the bundle channel asap since there are no downstream
	// collectors that depend on AD having up to date data.
	close(evBundle.Ch)

	for _, ev := range evBundle.Events {
		entity := ev.Entity
		entityID := entity.GetID()

		if entityID.Kind != workloadmeta.KindContainer {
			log.Errorf("got event %d with entity of kind %q. filters broken?", ev.Type, entityID.Kind)
		}

		switch ev.Type {
		case workloadmeta.EventTypeSet:
			container := entity.(*workloadmeta.Container)
			l.createContainerService(container)

		case workloadmeta.EventTypeUnset:
			l.removeService(entityID)

		default:
			log.Errorf("cannot handle event of type %d", ev.Type)
		}
	}
}

func (l *DockerListener) createContainerService(container *workloadmeta.Container) {
	containerImg := container.Image
	if l.filters.IsExcluded(
		containers.GlobalFilter,
		container.Name,
		containerImg.RawName,
		"",
	) {
		log.Debugf("container %s filtered out: name %q image %q", container.ID, container.Name, containerImg.RawName)
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

	var ports []ContainerPort
	for _, port := range container.Ports {
		ports = append(ports, ContainerPort{
			Port: port.Port,
			Name: port.Name,
		})
	}

	sort.Slice(ports, func(i, j int) bool {
		return ports[i].Port < ports[j].Port
	})

	dockerSvc := &DockerService{
		containerID:  container.ID,
		creationTime: integration.After,
		adIdentifiers: ComputeContainerServiceIDs(
			containers.BuildEntityName(string(container.Runtime), container.ID),
			containerImg.RawName,
			container.Labels,
		),
		ports:    ports,
		pid:      container.PID,
		hostname: container.Hostname,
	}

	var svc Service
	if findKubernetesInLabels(container.Labels) {
		kubeSvc := &DockerKubeletService{
			DockerService: *dockerSvc,
		}

		pod, err := l.store.GetKubernetesPodForContainer(container.ID)
		if err == nil {
			kubeSvc.hosts = map[string]string{"pod": pod.IP}
			kubeSvc.ready = pod.Ready
		} else {
			log.Debugf("container %q belongs to a pod but was not found: %s", container.ID, err)
		}

		svc = kubeSvc
	} else {
		checkNames, err := getCheckNamesFromLabels(container.Labels)
		if err != nil {
			log.Errorf("error getting check names from docker labels on container %s: %v", container.ID, err)
		}

		hosts := make(map[string]string)
		for host, ip := range container.NetworkIPs {
			hosts[host] = ip
		}

		if rancherIP, ok := docker.FindRancherIPInLabels(container.Labels); ok {
			hosts["rancher"] = rancherIP
		}

		// Some CNI solutions (including ECS awsvpc) do not assign an
		// IP through docker, but set a valid reachable hostname. Use
		// it if no IP is discovered.
		if len(hosts) == 0 && len(container.Hostname) > 0 {
			hosts["hostname"] = container.Hostname
		}

		dockerSvc.hosts = hosts
		dockerSvc.checkNames = checkNames
		dockerSvc.metricsExcluded = l.filters.IsExcluded(
			containers.MetricsFilter,
			container.Name,
			containerImg.RawName,
			"",
		)
		dockerSvc.logsExcluded = l.filters.IsExcluded(
			containers.LogsFilter,
			container.Name,
			containerImg.RawName,
			"",
		)

		svc = dockerSvc
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	svcID := buildSvcID(container.GetID())
	l.services[svcID] = svc
	l.newService <- svc
}

func (l *DockerListener) removeService(entityID workloadmeta.EntityID) {
	l.mu.Lock()
	defer l.mu.Unlock()

	svcID := buildSvcID(entityID)
	svc, ok := l.services[svcID]
	if !ok {
		log.Debugf("service %q not found, not removing", svcID)
		return
	}

	delete(l.services, svcID)
	l.delService <- svc
}

// Stop stops the DockerListener.
func (l *DockerListener) Stop() {
	l.stop <- struct{}{}
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
