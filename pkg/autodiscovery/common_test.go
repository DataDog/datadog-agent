// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package autodiscovery

import (
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
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
	CreationTime  integration.CreationTime
	CheckNames    []string
}

// GetEntity returns the service entity name
func (s *dummyService) GetEntity() string {
	return s.ID
}

// GetEntity returns the service entity name
func (s *dummyService) GetTaggerEntity() string {
	return s.ID
}

// GetADIdentifiers returns dummy identifiers
func (s *dummyService) GetADIdentifiers() ([]string, error) {
	return s.ADIdentifiers, nil
}

// GetHosts returns dummy hosts
func (s *dummyService) GetHosts() (map[string]string, error) {
	return s.Hosts, nil
}

// GetPorts returns dummy ports
func (s *dummyService) GetPorts() ([]listeners.ContainerPort, error) {
	return s.Ports, nil
}

// GetTags returns mil
func (s *dummyService) GetTags() ([]string, error) {
	return nil, nil
}

// GetPid return a dummy pid
func (s *dummyService) GetPid() (int, error) {
	return s.Pid, nil
}

// GetHostname return a dummy hostname
func (s *dummyService) GetHostname() (string, error) {
	return s.Hostname, nil
}

// GetCreationTime return a dummy creation time
func (s *dummyService) GetCreationTime() integration.CreationTime {
	return s.CreationTime
}

// IsReady returns if the service is ready
func (s *dummyService) IsReady() bool {
	return true
}

// GetCheckNames returns slice of check names defined in docker labels
func (s *dummyService) GetCheckNames() []string {
	return s.CheckNames
}

// HasFilter returns false
func (s *dummyService) HasFilter(filter containers.FilterType) bool {
	return false
}

// GetExtraConfig isn't supported
func (s *dummyService) GetExtraConfig(key []byte) ([]byte, error) {
	return []byte{}, nil
}
