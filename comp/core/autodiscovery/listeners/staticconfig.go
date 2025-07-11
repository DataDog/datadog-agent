// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package listeners

import (
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	filter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

// StaticConfigListener implements a ServiceListener based on static configuration parameters
type StaticConfigListener struct {
	newService chan<- Service
}

// StaticConfigService represents services generated from StaticConfigListener
type StaticConfigService struct {
	adIdentifier string
}

// Make sure StaticConfigService implements the Service interface
var _ Service = &StaticConfigService{}

// NewStaticConfigListener creates a StaticConfigListener
func NewStaticConfigListener(ServiceListernerDeps) (ServiceListener, error) {
	return &StaticConfigListener{}, nil
}

// Listen starts the goroutine to detect checks based on the config
func (l *StaticConfigListener) Listen(newSvc chan<- Service, _ chan<- Service) {
	l.newService = newSvc

	go l.createServices()
}

// Stop has nothing to do in this case
func (l *StaticConfigListener) Stop() {
}

func (l *StaticConfigListener) createServices() {
	for _, staticCheck := range []string{
		"container_image",
		"container_lifecycle",
		"sbom",
	} {
		if enabled := pkgconfigsetup.Datadog().GetBool(staticCheck + ".enabled"); enabled {
			l.newService <- &StaticConfigService{adIdentifier: "_" + staticCheck}
		}
	}

	if enabled := pkgconfigsetup.SystemProbe().GetBool("discovery.enabled"); enabled {
		l.newService <- &StaticConfigService{adIdentifier: "_discovery"}
	}
}

// Equal returns whether the two StaticConfigService are equal
func (s *StaticConfigService) Equal(o Service) bool {
	s2, ok := o.(*StaticConfigService)
	if !ok {
		return false
	}

	return s.adIdentifier == s2.adIdentifier
}

// GetServiceID returns the unique entity name linked to that service
func (s *StaticConfigService) GetServiceID() string {
	return s.adIdentifier
}

// GetADIdentifiers return the single AD identifier for a static config service
func (s *StaticConfigService) GetADIdentifiers() []string {
	return []string{s.adIdentifier}
}

// GetHosts is not supported
func (s *StaticConfigService) GetHosts() (map[string]string, error) {
	return nil, ErrNotSupported
}

// GetPorts returns nil and an error because port is not supported in this listener
func (s *StaticConfigService) GetPorts() ([]ContainerPort, error) {
	return nil, ErrNotSupported
}

// GetTags retrieves a container's tags
func (s *StaticConfigService) GetTags() ([]string, error) {
	return nil, nil
}

// GetTagsWithCardinality returns the tags with given cardinality.
func (s *StaticConfigService) GetTagsWithCardinality(_ string) ([]string, error) {
	return s.GetTags()
}

// GetPid inspect the container and return its pid
// Not relevant in this listener
func (s *StaticConfigService) GetPid() (int, error) {
	return -1, ErrNotSupported
}

// GetHostname returns nil and an error because port is not supported in this listener
func (s *StaticConfigService) GetHostname() (string, error) {
	return "", ErrNotSupported
}

// IsReady is always true
func (s *StaticConfigService) IsReady() bool {
	return true
}

// HasFilter is not supported
func (s *StaticConfigService) HasFilter(_ filter.Scope) bool {
	return false
}

// GetExtraConfig is not supported
func (s *StaticConfigService) GetExtraConfig(_ string) (string, error) {
	return "", ErrNotSupported
}

// FilterTemplates does nothing.
func (s *StaticConfigService) FilterTemplates(_ map[string]integration.Config) {
}
