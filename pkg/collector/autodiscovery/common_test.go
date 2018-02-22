// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package autodiscovery

import "github.com/DataDog/datadog-agent/pkg/collector/listeners"

type dummyService struct {
	ID            listeners.ID
	ADIdentifiers []string
	Hosts         map[string]string
	Ports         []int
	Pid           int
}

// GetID returns the service ID
func (s *dummyService) GetID() listeners.ID {
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
func (s *dummyService) GetPorts() ([]int, error) {
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
