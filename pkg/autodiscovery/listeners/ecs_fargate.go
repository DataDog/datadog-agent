// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

// +build !serverless

package listeners

import (
	"context"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

// ECSService implements and store results from the Service interface for the
// ECS Fargate listener
type ECSService struct {
	cID             string
	runtime         string
	ADIdentifiers   []string
	hosts           map[string]string
	clusterName     string
	taskFamily      string
	taskVersion     string
	creationTime    integration.CreationTime
	checkNames      []string
	metricsExcluded bool
	logsExcluded    bool
}

// Make sure ECSService implements the Service interface
var _ Service = &ECSService{}

func init() {
	Register("ecs", NewECSFargateListener)
}

// ECSFargateListener listens to container creation through a subscription to the
// workloadmeta store.
type ECSFargateListener struct {
	store *workloadmeta.Store
	stop  chan struct{}

	mu             sync.RWMutex
	filters        *containerFilters
	services       map[string]Service
	taskContainers map[string]map[string]struct{}

	newService chan<- Service
	delService chan<- Service
}

// NewECSFargateListener returns a new ECSFargateListener.
func NewECSFargateListener() (ServiceListener, error) {
	filters, err := newContainerFilters()
	if err != nil {
		return nil, err
	}

	return &ECSFargateListener{
		store:          workloadmeta.GetGlobalStore(),
		filters:        filters,
		services:       make(map[string]Service),
		stop:           make(chan struct{}),
		taskContainers: make(map[string]map[string]struct{}),
	}, nil
}

// Listen starts listening to events from the workloadmeta store.
func (l *ECSFargateListener) Listen(newSvc chan<- Service, delSvc chan<- Service) {
	l.newService = newSvc
	l.delService = delSvc

	const name = "ad-ecsfargatelistener"

	ch := l.store.Subscribe(name, workloadmeta.NewFilter(
		[]workloadmeta.Kind{workloadmeta.KindECSTask},
		[]string{"ecs_fargate"},
	))
	health := health.RegisterLiveness(name)
	firstRun := true

	log.Info("ecs fargate listener initialized successfully")

	go func() {
		for {
			select {
			case evBundle := <-ch:
				l.processEvents(evBundle, firstRun)
				firstRun = false

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

func (l *ECSFargateListener) processEvents(evBundle workloadmeta.EventBundle, firstRun bool) {
	// close the bundle channel asap since there are no downstream
	// collectors that depend on AD having up to date data.
	close(evBundle.Ch)

	for _, ev := range evBundle.Events {
		entity := ev.Entity
		entityID := entity.GetID()

		if entityID.Kind != workloadmeta.KindECSTask {
			log.Errorf("internal error: got event %d with entity of kind %q. filters broken?", ev.Type, entityID.Kind)
			continue
		}

		switch ev.Type {
		case workloadmeta.EventTypeSet:
			task := entity.(*workloadmeta.ECSTask)
			l.processTask(task, firstRun)

		case workloadmeta.EventTypeUnset:
			l.removeTaskService(entityID)

		default:
			log.Errorf("cannot handle event of type %d", ev.Type)
		}
	}
}

func (l *ECSFargateListener) processTask(task *workloadmeta.ECSTask, firstRun bool) {
	// unseen keeps track of which previous container services are no
	// longer present in the task, to be removed at the end of this func
	svcID := buildSvcID(task.GetID())
	unseen := make(map[string]struct{})
	for id := range l.taskContainers[svcID] {
		unseen[id] = struct{}{}
	}

	containers := make([]*workloadmeta.Container, 0, len(task.Containers))
	for _, taskContainer := range task.Containers {
		container, err := l.store.GetContainer(taskContainer.ID)
		if err != nil {
			log.Debugf("task %q has reference to non-existing container %q", task.Name, taskContainer.ID)
			continue
		}

		l.createContainerService(task, container, firstRun)

		containers = append(containers, container)

		containerSvcID := buildSvcID(container.GetID())
		delete(unseen, containerSvcID)
	}

	// remove the container services that weren't seen when processing this
	// task
	for containerSvcID := range unseen {
		l.removeService(containerSvcID)
	}
}

func (l *ECSFargateListener) createContainerService(task *workloadmeta.ECSTask, container *workloadmeta.Container, firstRun bool) {
	if !container.State.Running {
		return
	}

	containerImg := container.Image
	if l.filters.IsExcluded(
		containers.GlobalFilter,
		container.Name,
		containerImg.RawName,
		"",
	) {
		log.Debugf("container %s filtered out: name %q image %q", container.ID, container.Name, container.Image.RawName)
		return
	}

	var crTime integration.CreationTime
	if firstRun {
		crTime = integration.Before
	} else {
		crTime = integration.After
	}

	hosts := make(map[string]string)
	for host, ip := range container.NetworkIPs {
		hosts[host] = ip
	}

	svc := &ECSService{
		cID: container.ID,
		ADIdentifiers: ComputeContainerServiceIDs(
			containers.BuildEntityName(string(container.Runtime), container.ID),
			containerImg.RawName,
			container.Labels,
		),
		runtime:      string(container.Runtime),
		clusterName:  task.ClusterName,
		taskFamily:   task.Family,
		taskVersion:  task.Version,
		creationTime: crTime,
		hosts:        hosts,
		metricsExcluded: l.filters.IsExcluded(
			containers.MetricsFilter,
			container.Name,
			containerImg.RawName,
			"",
		),
		logsExcluded: l.filters.IsExcluded(
			containers.LogsFilter,
			container.Name,
			containerImg.RawName,
			"",
		),
	}

	var err error
	svc.checkNames, err = getCheckNamesFromLabels(container.Labels)
	if err != nil {
		log.Errorf("error getting check names from docker labels on container %s: %v", container.ID, err)
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	svcID := buildSvcID(container.GetID())
	taskSvcID := buildSvcID(task.GetID())

	if _, ok := l.taskContainers[taskSvcID]; !ok {
		l.taskContainers[taskSvcID] = make(map[string]struct{})
	}

	l.services[svcID] = svc
	l.taskContainers[taskSvcID][svcID] = struct{}{}
	l.newService <- svc
}

func (l *ECSFargateListener) removeTaskService(entityID workloadmeta.EntityID) {
	svcID := buildSvcID(entityID)

	l.mu.Lock()
	containerSvcIDs := l.taskContainers[svcID]
	delete(l.taskContainers, svcID)
	l.mu.Unlock()

	for containerSvcID := range containerSvcIDs {
		l.removeService(containerSvcID)
	}
}

func (l *ECSFargateListener) removeService(svcID string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	svc, ok := l.services[svcID]
	if !ok {
		log.Debugf("service %q not found, not removing", svcID)
		return
	}

	delete(l.services, svcID)
	l.delService <- svc
}

// Stop stops the ECSFargateListener.
func (l *ECSFargateListener) Stop() {
	l.stop <- struct{}{}
}

// GetEntity returns the unique entity name linked to that service
func (s *ECSService) GetEntity() string {
	return containers.BuildEntityName(s.runtime, s.cID)
}

// GetTaggerEntity returns the tagger entity ID for the service.
func (s *ECSService) GetTaggerEntity() string {
	return containers.BuildTaggerEntityName(s.cID)
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
func (s *ECSService) GetADIdentifiers(context.Context) ([]string, error) {
	return s.ADIdentifiers, nil
}

// GetHosts returns the container's hosts
// TODO: using localhost should usually be enough
func (s *ECSService) GetHosts(context.Context) (map[string]string, error) {
	return s.hosts, nil
}

// GetPorts returns nil and an error because port is not supported in Fargate-based ECS
func (s *ECSService) GetPorts(context.Context) ([]ContainerPort, error) {
	return nil, ErrNotSupported
}

// GetTags retrieves a container's tags
func (s *ECSService) GetTags() ([]string, string, error) {
	return tagger.TagWithHash(s.GetTaggerEntity(), tagger.ChecksCardinality)
}

// GetPid inspect the container and return its pid
// TODO: not supported as pid is not in the metadata api
func (s *ECSService) GetPid(context.Context) (int, error) {
	return -1, ErrNotSupported
}

// GetHostname returns nil and an error because port is not supported in Fargate-based ECS
func (s *ECSService) GetHostname(context.Context) (string, error) {
	return "", ErrNotSupported
}

// GetCreationTime returns the creation time of the container compare to the agent start.
func (s *ECSService) GetCreationTime() integration.CreationTime {
	return s.creationTime
}

// IsReady returns whether the service is ready or not.
func (s *ECSService) IsReady(context.Context) bool {
	return true
}

// GetCheckNames returns slice check names defined in docker labels
func (s *ECSService) GetCheckNames(context.Context) []string {
	return s.checkNames
}

// HasFilter returns true if metrics or logs collection must be excluded for this service
// no containers.GlobalFilter case here because we don't create services that are globally excluded in AD
func (s *ECSService) HasFilter(filter containers.FilterType) bool {
	switch filter {
	case containers.MetricsFilter:
		return s.metricsExcluded
	case containers.LogsFilter:
		return s.logsExcluded
	}
	return false
}

// GetExtraConfig isn't supported
func (s *ECSService) GetExtraConfig(key []byte) ([]byte, error) {
	return []byte{}, ErrNotSupported
}
