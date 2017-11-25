// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package listeners

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	log "github.com/cihub/seelog"
)

const (
	metadataURL string = "http://169.254.170.2/v2/metadata"
)

// ignore these container labels as we already have them in task metadata
var labelBlackList = map[string]interface{}{
	"com.amazonaws.ecs.cluster":                 nil,
	"com.amazonaws.ecs.container-name":          nil,
	"com.amazonaws.ecs.task-arn":                nil,
	"com.amazonaws.ecs.task-definition-family":  nil,
	"com.amazonaws.ecs.task-definition-version": nil,
}

// ECSListener implements the ServiceListener interface.
// It pulls its tasks container list periodically and checks for
// new containers to monitor, and old containers to stop monitoring
type ECSListener struct {
	client     http.Client
	task       TaskMetadata
	services   map[string]Service // maps container IDs to services
	newService chan<- Service
	delService chan<- Service
	stop       chan bool
	m          sync.RWMutex
	t          *time.Ticker
}

// ECSService implements and store results from the Service interface for the ECS listener
type ECSService struct {
	ID            ID
	ADIdentifiers []string
	Hosts         map[string]string
	Ports         []int
	Pid           int
	Tags          []string
	ecsContainer  ECSContainer
	clusterName   string
	taskFamily    string
	taskVersion   string
}

// TaskMetadata is the info returned by the ECS task metadata API
type TaskMetadata struct {
	ClusterName   string         `json:"Cluster"`
	Containers    []ECSContainer `json:"Containers"`
	KnownStatus   string         `json:"KnownStatus"`
	TaskARN       string         `json:"TaskARN"`
	Family        string         `json:"Family"`
	Version       string         `json:"Version"`
	Limits        map[string]int `json:"Limits"`
	DesiredStatus string         `json:"DesiredStatus"`
}

// ECSContainer is the representation of a container as exposed by the ECS metadata API
type ECSContainer struct {
	Name          string            `json:"Name"`
	Limits        map[string]int    `json:"Limits"`
	ImageID       string            `json:"ImageID,omitempty"`
	StartedAt     string            `json:"StartedAt"` // 2017-11-17T17:14:07.781711848Z
	DockerName    string            `json:"DockerName"`
	Type          string            `json:"Type"`
	Image         string            `json:"Image"`
	Labels        map[string]string `json:"Labels"`
	KnownStatus   string            `json:"KnownStatus"`
	DesiredStatus string            `json:"DesiredStatus"`
	DockerID      string            `json:"DockerID"`
	CreatedAt     string            `json:"CreatedAt"`
	Networks      []ECSNetwork      `json:"Networks"`
	Ports         string            `json:"Ports"` // TODO: support it. It's not strictly needed for now
}

// ECSNetwork represents the network of a container
type ECSNetwork struct {
	NetworkMode   string   `json:"NetworkMode"`   // as of today the only supported mode is awsvpc
	IPv4Addresses []string `json:"IPv4Addresses"` // one-element list
}

func init() {
	log.Infof("registering ecs listener")
	Register("ecs", NewECSListener)
}

// NewECSListener creates an ECSListener
func NewECSListener() (ServiceListener, error) {
	c := http.Client{
		Timeout: 500 * time.Millisecond,
	}
	return &ECSListener{
		client:   c,
		services: make(map[string]Service),
		stop:     make(chan bool),
		t:        time.NewTicker(2 * time.Second),
	}, nil
}

// Listen polls regularly container-related events from the ECS task metadata endpoint and report said containers as Services.
func (l *ECSListener) Listen(newSvc chan<- Service, delSvc chan<- Service) {
	// setup the I/O channels
	l.newService = newSvc
	l.delService = delSvc

	log.Infof("starting listening to ECS services")
	go func() {
		for {
			select {
			case <-l.t.C:
				l.refreshServices()
			case <-l.stop:
				return
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
	log.Infof("refreshing services...")
	meta, err := l.getTaskMetadata()
	if err != nil {
		log.Errorf("failed to get task metadata, not refreshing services - %s", err)
		return
	} else if meta.KnownStatus != "RUNNING" {
		log.Errorf("task %s is not in RUNNING state yet, not refreshing services", meta.Family)
	}
	l.task = meta

	// if not found and running, add it. Else no-op
	// at the end, compare what we saw and what is cached and kill what's not there anymore
	notSeen := make(map[string]interface{})
	for i := range l.services {
		notSeen[i] = nil
	}
	for _, c := range meta.Containers {
		if _, found := l.services[c.DockerID]; !found {
			if c.KnownStatus == "RUNNING" {
				s, err := l.createService(c)
				if err != nil {
					log.Errorf("couldn't create a service out of container %s - Auto Discovery will ignore it", c.DockerID)
				} else {
					l.services[c.DockerID] = &s
					log.Infof("Submitting new service: %s", s.ADIdentifiers)
					l.newService <- &s
					delete(notSeen, c.DockerID)
				}
			} else {
				log.Errorf("container %s is in status %s - skipping", c.DockerID, c.KnownStatus) // TODO: remove or move to debug
			}
		} else {
			log.Errorf("already know container %s, skipping", c.DockerID) // TODO: delete me
			delete(notSeen, c.DockerID)
		}
	}
	for cID := range notSeen {
		l.delService <- l.services[cID]
		delete(l.services, cID)
	}
}

func (l *ECSListener) getTaskMetadata() (TaskMetadata, error) {
	var meta TaskMetadata
	log.Infof("Getting task metadata...") // TODO: delete me
	resp, err := http.Get(metadataURL)
	if err != nil {
		return meta, err
	}
	defer resp.Body.Close()

	decoder := json.NewDecoder(resp.Body)
	err = decoder.Decode(&meta)
	if err != nil {
		log.Errorf("decoding failed!") // TODO: delete me
		return meta, err
	}
	return meta, err
}

func (l *ECSListener) createService(c ECSContainer) (ECSService, error) {
	log.Infof("creating service...")
	cID := ID(c.DockerID)
	svc := ECSService{
		ID:           cID,
		ecsContainer: c,
		clusterName:  l.task.ClusterName,
		taskFamily:   l.task.Family,
		taskVersion:  l.task.Version,
	}
	_, err := svc.GetADIdentifiers()
	if err != nil {
		log.Errorf("Failed to extract info for container %s - %s", cID[:12], err)
	}
	_, err = svc.GetHosts()
	if err != nil {
		log.Errorf("Failed to extract info for container %s - %s", cID[:12], err)
	}
	_, err = svc.GetPorts()
	if err != nil {
		log.Errorf("Failed to extract info for container %s - %s", cID[:12], err)
	}
	_, err = svc.GetPid()
	if err != nil {
		log.Errorf("Failed to extract info for container %s - %s", cID[:12], err)
	}
	_, err = svc.GetTags()
	if err != nil {
		log.Errorf("Failed to extract info for container %s - %s", cID[:12], err)
	}
	log.Infof("SERVICE:")
	log.Infof("cluster: %s", svc.clusterName)
	log.Infof("identifiers: %s", svc.ADIdentifiers)
	log.Infof("family: %s", svc.taskFamily)
	log.Infof("version: %s", svc.taskVersion)
	log.Infof("container: %s", svc.ecsContainer.DockerName)
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
	if len(s.ADIdentifiers) == 0 {
		cID := s.ecsContainer.DockerID
		image := s.ecsContainer.Image
		labels := s.ecsContainer.Labels
		s.ADIdentifiers = ComputeContainerServiceIDs(cID, image, labels)
	}

	return s.ADIdentifiers, nil
}

// GetHosts returns the container's hosts
// TODO: using localhost should be enough in most cases
func (s *ECSService) GetHosts() (map[string]string, error) {
	if s.Hosts != nil {
		return s.Hosts, nil
	}

	ips := make(map[string]string)

	for _, net := range s.ecsContainer.Networks {
		if net.NetworkMode == "awsvpc" && len(net.IPv4Addresses) > 0 {
			ips["awsvpc"] = string(net.IPv4Addresses[0])
		}
	}

	s.Hosts = ips
	return ips, nil
}

// GetPorts returns the container's ports
// TODO: not supported yet, this is a place holder
func (s *ECSService) GetPorts() ([]int, error) {
	if s.Ports == nil {
		ports := make([]int, 0)
		s.Ports = ports
	}

	return s.Ports, nil
}

// GetTags retrieves a container's tags
// TODO: move it to tagger
func (s *ECSService) GetTags() ([]string, error) {
	if len(s.Tags) > 0 {
		return s.Tags, nil
	}
	var tags []string

	// cluster
	tags = append(tags, fmt.Sprintf("ecs_cluster_name:%s", s.clusterName))

	// task
	tags = append(tags, fmt.Sprintf("ecs_task_family:%s", s.taskFamily))
	tags = append(tags, fmt.Sprintf("ecs_task_version:%s", s.taskVersion))

	// container
	tags = append(tags, fmt.Sprintf("ecs_container_name:%s", s.ecsContainer.Name))
	tags = append(tags, fmt.Sprintf("docker_container_name::%s", s.ecsContainer.DockerName))

	// container image
	image := s.ecsContainer.Image
	tags = append(tags, fmt.Sprintf("docker_image:%s", image))
	imageSplit := strings.Split(image, ":")
	imageName := strings.Join(imageSplit[:len(imageSplit)-1], ":")
	tags = append(tags, fmt.Sprintf("imageName:%s", imageName))
	if len(imageSplit) > 1 {
		imageTag := imageSplit[len(imageSplit)-1]
		tags = append(tags, fmt.Sprintf("image_tag:%s", imageTag))
	}

	// container labels
	for k, v := range s.ecsContainer.Labels {
		if _, found := labelBlackList[k]; !found {
			tags = append(tags, fmt.Sprintf("%s:%s", k, v))
		}
	}

	s.Tags = tags
	return tags, nil
}

// GetPid inspect the container an return its pid
func (s *ECSService) GetPid() (int, error) {
	// TODO: not available in the metadata api yet
	s.Pid = 2
	return 2, nil
}
