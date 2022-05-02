// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package autodiscovery

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/listeners"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

type dummyService struct {
	ID            string
	ADIdentifiers []string
	Hosts         map[string]string
	Ports         []listeners.ContainerPort
	Pid           int
	Hostname      string
	CheckNames    []string
}

// GetServiceID returns the service entity name
func (s *dummyService) GetServiceID() string {
	return s.ID
}

// GetTaggerEntity returns the tagger entity ID for the entity corresponding to this service
func (s *dummyService) GetTaggerEntity() string {
	return s.ID
}

// GetADIdentifiers returns dummy identifiers
func (s *dummyService) GetADIdentifiers(context.Context) ([]string, error) {
	return s.ADIdentifiers, nil
}

// GetHosts returns dummy hosts
func (s *dummyService) GetHosts(context.Context) (map[string]string, error) {
	return s.Hosts, nil
}

// GetPorts returns dummy ports
func (s *dummyService) GetPorts(context.Context) ([]listeners.ContainerPort, error) {
	return s.Ports, nil
}

// GetTags returns the tags for this service
func (s *dummyService) GetTags() ([]string, error) {
	return nil, nil
}

// GetPid return a dummy pid
func (s *dummyService) GetPid(context.Context) (int, error) {
	return s.Pid, nil
}

// GetHostname return a dummy hostname
func (s *dummyService) GetHostname(context.Context) (string, error) {
	return s.Hostname, nil
}

// IsReady returns if the service is ready
func (s *dummyService) IsReady(context.Context) bool {
	return true
}

// GetCheckNames returns slice of check names defined in container labels
func (s *dummyService) GetCheckNames(context.Context) []string {
	return s.CheckNames
}

// HasFilter returns false
func (s *dummyService) HasFilter(filter containers.FilterType) bool {
	return false
}

// GetExtraConfig isn't supported
func (s *dummyService) GetExtraConfig(key string) (string, error) {
	return "", nil
}
