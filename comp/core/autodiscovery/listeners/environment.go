// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

package listeners

import (
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	filter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// EnvironmentListener implements a ServiceListener based on current environment
type EnvironmentListener struct {
	newService chan<- Service
}

// EnvironmentService represents services generated from EnvironmentListener
type EnvironmentService struct {
	adIdentifier string
}

// Make sure EnvironmentService implements the Service interface
var _ Service = &EnvironmentService{}

// NewEnvironmentListener creates an EnvironmentListener
func NewEnvironmentListener(ServiceListernerDeps) (ServiceListener, error) {
	return &EnvironmentListener{}, nil
}

// Listen starts the goroutine to detect checks based on environment
func (l *EnvironmentListener) Listen(newSvc chan<- Service, _ chan<- Service) {
	l.newService = newSvc

	// ATM we consider environment as a fixed space
	// It may change in the future
	go l.createServices()
}

// Stop has nothing to do in this case
func (l *EnvironmentListener) Stop() {
}

func (l *EnvironmentListener) createServices() {
	features := map[string]env.Feature{
		"docker":            env.Docker,
		"kubelet":           env.Kubernetes,
		"ecs_fargate":       env.ECSFargate,
		"eks_fargate":       env.EKSFargate,
		"cri":               env.Cri,
		"containerd":        env.Containerd,
		"kube_orchestrator": env.KubeOrchestratorExplorer,
		"ecs_orchestrator":  env.ECSOrchestratorExplorer,
	}

	for name, feature := range features {
		if env.IsFeaturePresent(feature) {
			log.Infof("Listener created %s service from environment", name)
			l.newService <- &EnvironmentService{adIdentifier: "_" + name}
		}
	}

	// Handle generic container check auto-activation.
	if env.IsAnyContainerFeaturePresent() {
		log.Infof("Listener created container service from environment")
		l.newService <- &EnvironmentService{adIdentifier: "_container"}
	}
}

// Equal returns whether the two EnvironmentService are equal
func (s *EnvironmentService) Equal(o Service) bool {
	s2, ok := o.(*EnvironmentService)
	if !ok {
		return false
	}

	return s.adIdentifier == s2.adIdentifier
}

// GetServiceID returns the unique entity name linked to that service
func (s *EnvironmentService) GetServiceID() string {
	return s.adIdentifier
}

// GetADIdentifiers return the single AD identifier for an environment service
func (s *EnvironmentService) GetADIdentifiers() []string {
	return []string{s.adIdentifier}
}

// GetHosts is not supported
func (s *EnvironmentService) GetHosts() (map[string]string, error) {
	return nil, ErrNotSupported
}

// GetPorts returns nil and an error because port is not supported in this listener
func (s *EnvironmentService) GetPorts() ([]ContainerPort, error) {
	return nil, ErrNotSupported
}

// GetTags retrieves a container's tags
func (s *EnvironmentService) GetTags() ([]string, error) {
	return nil, nil
}

// GetTagsWithCardinality returns the tags with given cardinality. Not supported in EnvironmentService
func (s *EnvironmentService) GetTagsWithCardinality(_ string) ([]string, error) {
	return s.GetTags()
}

// GetPid inspect the container and return its pid
// Not relevant in this listener
func (s *EnvironmentService) GetPid() (int, error) {
	return -1, ErrNotSupported
}

// GetHostname returns nil and an error because port is not supported in this listener
func (s *EnvironmentService) GetHostname() (string, error) {
	return "", ErrNotSupported
}

// IsReady is always true
func (s *EnvironmentService) IsReady() bool {
	return true
}

// HasFilter is not supported
func (s *EnvironmentService) HasFilter(_ filter.Scope) bool {
	return false
}

// GetExtraConfig is not supported
func (s *EnvironmentService) GetExtraConfig(_ string) (string, error) {
	return "", ErrNotSupported
}

// FilterTemplates does nothing.
func (s *EnvironmentService) FilterTemplates(_ map[string]integration.Config) {
}
