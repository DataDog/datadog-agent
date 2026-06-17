// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

package listeners

import (
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	sysprobeconfig "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/def"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// Checks activated from configuration state (avoid double work to activate it for users)
var sysProbeConfigChecks = []struct {
	adIdentifier string
	configKey    string
}{
	{adIdentifier: "_oom_kill", configKey: "system_probe_config.enable_oom_kill"},
	{adIdentifier: "_tcp_queue_length", configKey: "system_probe_config.enable_tcp_queue_length"},
}

// EnvironmentListener implements a ServiceListener based on current environment
type EnvironmentListener struct {
	newService     chan<- Service
	sysProbeConfig option.Option[sysprobeconfig.Component]
}

// EnvironmentService represents services generated from EnvironmentListener
type EnvironmentService struct {
	adIdentifier string
}

// Make sure EnvironmentService implements the Service interface
var _ Service = &EnvironmentService{}

// NewEnvironmentListener creates an EnvironmentListener
func NewEnvironmentListener(deps ServiceListernerDeps) (ServiceListener, error) {
	return &EnvironmentListener{sysProbeConfig: deps.SysProbeConfig}, nil
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
		"docker":                      env.Docker,
		"kubelet":                     env.Kubernetes,
		"ecs_fargate":                 env.ECSFargate,
		"eks_fargate":                 env.EKSFargate,
		"cri":                         env.Cri,
		"containerd":                  env.Containerd,
		"kube_orchestrator":           env.KubeOrchestratorExplorer,
		"kubelet_config_orchestrator": env.KubeletConfigOrchestratorCheck,
		"ecs_orchestrator":            env.ECSOrchestratorExplorer,
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

	// Handle checks auto-activated from system-probe configuration state.
	if sysProbeConfig, ok := l.sysProbeConfig.Get(); ok {
		for _, check := range sysProbeConfigChecks {
			if sysProbeConfig.GetBool(check.configKey) {
				log.Infof("Listener created %s service from system-probe configuration", check.adIdentifier)
				l.newService <- &EnvironmentService{adIdentifier: check.adIdentifier}
			}
		}
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
func (s *EnvironmentService) GetPorts() ([]workloadmeta.ContainerPort, error) {
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
func (s *EnvironmentService) HasFilter(_ workloadfilter.Scope) bool {
	return false
}

// GetExtraConfig is not supported
func (s *EnvironmentService) GetExtraConfig(_ string) (string, error) {
	return "", ErrNotSupported
}

// FilterTemplates does nothing.
func (s *EnvironmentService) FilterTemplates(_ map[string]integration.Config) {
}

// GetImageName does nothing
func (s *EnvironmentService) GetImageName() string {
	return ""
}
