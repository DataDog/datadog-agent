// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package autodiscoveryimpl

import (
	"context"
	"reflect"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/listeners"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

type dummyService struct {
	ID              string
	ADIdentifiers   []string
	Hosts           map[string]string
	Ports           []listeners.ContainerPort
	Pid             int
	Hostname        string
	filterTemplates func(map[string]integration.Config)
}

// Equal returns whether the two dummyService are equal
func (s *dummyService) Equal(o listeners.Service) bool {
	return reflect.DeepEqual(s, o)
}

// GetServiceID returns the service entity name
func (s *dummyService) GetServiceID() string {
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

// GetTagsWithCardinality returns the tags for this service
func (s *dummyService) GetTagsWithCardinality(_ string) ([]string, error) {
	return s.GetTags()
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

// HasFilter returns false
//
//nolint:revive // TODO(AML) Fix revive linter
func (s *dummyService) HasFilter(_ containers.FilterType) bool {
	return false
}

// GetExtraConfig isn't supported
//
//nolint:revive // TODO(AML) Fix revive linter
func (s *dummyService) GetExtraConfig(_ string) (string, error) {
	return "", nil
}

// FilterTemplates calls filterTemplates, if not nil
func (s *dummyService) FilterTemplates(configs map[string]integration.Config) {
	if s.filterTemplates != nil {
		(s.filterTemplates)(configs)
	}
}
