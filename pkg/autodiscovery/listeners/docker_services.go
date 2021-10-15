// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package listeners

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

// DockerService implements and store results from the Service interface for the Docker listener
type DockerService struct {
	containerID     string
	adIdentifiers   []string
	hosts           map[string]string
	ports           []ContainerPort
	pid             int
	hostname        string
	creationTime    integration.CreationTime
	checkNames      []string
	metricsExcluded bool
	logsExcluded    bool
}

// Make sure DockerService implements the Service interface
var _ Service = &DockerService{}

// GetEntity returns the unique entity name linked to that service
func (s DockerService) GetEntity() string {
	return containers.BuildEntityName(containers.RuntimeNameDocker, s.containerID)
}

// GetTaggerEntity returns the unique entity name linked to that service
func (s DockerService) GetTaggerEntity() string {
	return containers.BuildTaggerEntityName(s.containerID)
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
func (s DockerService) GetADIdentifiers(ctx context.Context) ([]string, error) {
	return s.adIdentifiers, nil
}

// GetHosts returns the container's hosts
func (s DockerService) GetHosts(ctx context.Context) (map[string]string, error) {
	return s.hosts, nil
}

// GetPorts returns the container's ports
func (s DockerService) GetPorts(ctx context.Context) ([]ContainerPort, error) {
	return s.ports, nil
}

// GetTags retrieves tags using the Tagger
func (s DockerService) GetTags() ([]string, string, error) {
	return tagger.TagWithHash(s.GetTaggerEntity(), tagger.ChecksCardinality)
}

// GetPid inspect the container an return its pid
func (s DockerService) GetPid(ctx context.Context) (int, error) {
	return s.pid, nil
}

// GetHostname returns hostname.domainname for the container
func (s DockerService) GetHostname(ctx context.Context) (string, error) {
	return s.hostname, nil
}

// GetCreationTime returns the creation time of the container compare to the agent start.
func (s DockerService) GetCreationTime() integration.CreationTime {
	return s.creationTime
}

// IsReady returns if the service is ready
func (s DockerService) IsReady(context.Context) bool {
	return true
}

// GetCheckNames returns slice check names defined in docker labels
func (s DockerService) GetCheckNames(ctx context.Context) []string {
	return s.checkNames
}

// HasFilter returns true if metrics or logs collection must be excluded for this service
// no containers.GlobalFilter case here because we don't create services that are globally excluded in AD
func (s DockerService) HasFilter(filter containers.FilterType) bool {
	switch filter {
	case containers.MetricsFilter:
		return s.metricsExcluded
	case containers.LogsFilter:
		return s.logsExcluded
	}
	return false
}

// GetExtraConfig isn't supported
func (s DockerService) GetExtraConfig(key []byte) ([]byte, error) {
	return []byte{}, ErrNotSupported
}

// DockerKubeletService overrides some methods when a container is running in
// kubernetes
type DockerKubeletService struct {
	DockerService
	ready bool
}

// Make sure DockerKubeletService implements the Service interface
var _ Service = &DockerKubeletService{}

// HasFilter always returns false
// DockerKubeletService doesn't implement this method
func (s DockerKubeletService) HasFilter(filter containers.FilterType) bool {
	return false
}

// IsReady returns if the service is ready
func (s DockerKubeletService) IsReady(context.Context) bool {
	return s.ready
}

// GetCheckNames returns slice of check names defined in kubernetes annotations or docker labels
// DockerKubeletService doesn't implement this method
func (s DockerKubeletService) GetCheckNames(context.Context) []string {
	return nil
}
