// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

package listeners

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
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

func init() {
	Register("environment", NewEnvironmentListener)
}

// NewEnvironmentListener creates an EnvironmentListener
func NewEnvironmentListener(Config) (ServiceListener, error) {
	return &EnvironmentListener{}, nil
}

// Listen starts the goroutine to detect checks based on environment
func (l *EnvironmentListener) Listen(newSvc chan<- Service, delSvc chan<- Service) {
	l.newService = newSvc

	// ATM we consider environment as a fixed space
	// It may change in the future
	go l.createServices()
}

// Stop has nothing to do in this case
func (l *EnvironmentListener) Stop() {
}

func (l *EnvironmentListener) createServices() {
	features := map[string]config.Feature{
		"docker":            config.Docker,
		"kubelet":           config.Kubernetes,
		"ecs_fargate":       config.ECSFargate,
		"eks_fargate":       config.EKSFargate,
		"cri":               config.Cri,
		"containerd":        config.Containerd,
		"kube_orchestrator": config.KubeOrchestratorExplorer,
	}

	for name, feature := range features {
		if config.IsFeaturePresent(feature) {
			log.Infof("Listener created %s service from environment", name)
			l.newService <- &EnvironmentService{adIdentifier: "_" + name}
		}
	}

	// Handle generic container check auto-activation.
	if config.IsAnyContainerFeaturePresent() {
		log.Infof("Listener created container service from environment")
		l.newService <- &EnvironmentService{adIdentifier: "_container"}
	}
}

// GetServiceID returns the unique entity name linked to that service
func (s *EnvironmentService) GetServiceID() string {
	return s.adIdentifier
}

// GetTaggerEntity returns the tagger entity
func (s *EnvironmentService) GetTaggerEntity() string {
	return ""
}

// GetADIdentifiers return the single AD identifier for an environment service
func (s *EnvironmentService) GetADIdentifiers(context.Context) ([]string, error) {
	return []string{s.adIdentifier}, nil
}

// GetHosts is not supported
func (s *EnvironmentService) GetHosts(context.Context) (map[string]string, error) {
	return nil, ErrNotSupported
}

// GetPorts returns nil and an error because port is not supported in this listener
func (s *EnvironmentService) GetPorts(context.Context) ([]ContainerPort, error) {
	return nil, ErrNotSupported
}

// GetTags retrieves a container's tags
func (s *EnvironmentService) GetTags() ([]string, error) {
	return nil, nil
}

// GetPid inspect the container and return its pid
// Not relevant in this listener
func (s *EnvironmentService) GetPid(context.Context) (int, error) {
	return -1, ErrNotSupported
}

// GetHostname returns nil and an error because port is not supported in this listener
func (s *EnvironmentService) GetHostname(context.Context) (string, error) {
	return "", ErrNotSupported
}

// IsReady is always true
func (s *EnvironmentService) IsReady(context.Context) bool {
	return true
}

// GetCheckNames is not supported
func (s *EnvironmentService) GetCheckNames(context.Context) []string {
	return nil
}

// HasFilter is not supported
func (s *EnvironmentService) HasFilter(filter containers.FilterType) bool {
	return false
}

// GetExtraConfig is not supported
func (s *EnvironmentService) GetExtraConfig(key string) (string, error) {
	return "", ErrNotSupported
}
