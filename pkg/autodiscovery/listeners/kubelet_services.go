// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package listeners

import (
	"context"
	"fmt"
	"reflect"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

// KubeContainerService implements and store results from the Service interface for the Kubelet listener
type KubeContainerService struct {
	entity          string
	adIdentifiers   []string
	hosts           map[string]string
	ports           []ContainerPort
	creationTime    integration.CreationTime
	ready           bool
	checkNames      []string
	metricsExcluded bool
	logsExcluded    bool
	extraConfig     map[string]string
}

// Make sure KubeContainerService implements the Service interface
var _ Service = &KubeContainerService{}

// KubePodService registers pod as a Service, implements and store results from the Service interface for the Kubelet listener
// needed to run checks on pod's endpoints
type KubePodService struct {
	entity        string
	adIdentifiers []string
	hosts         map[string]string
	ports         []ContainerPort
	creationTime  integration.CreationTime
}

// Make sure KubePodService implements the Service interface
var _ Service = &KubePodService{}

// kubeletSvcEqual returns false if one of the following fields aren't equal
// - hosts
// - ports
// - ad identifiers
// - check names
// - readiness
func kubeletSvcEqual(first, second Service) bool {
	ctx := context.TODO()

	hosts1, _ := first.GetHosts(ctx)
	hosts2, _ := second.GetHosts(ctx)
	if !reflect.DeepEqual(hosts1, hosts2) {
		return false
	}

	ports1, _ := first.GetPorts(ctx)
	ports2, _ := second.GetPorts(ctx)
	if !reflect.DeepEqual(ports1, ports2) {
		return false
	}

	ad1, _ := first.GetADIdentifiers(ctx)
	ad2, _ := second.GetADIdentifiers(ctx)
	if !reflect.DeepEqual(ad1, ad2) {
		return false
	}

	if !reflect.DeepEqual(first.GetCheckNames(ctx), second.GetCheckNames(ctx)) {
		return false
	}

	return first.IsReady(ctx) == second.IsReady(ctx)
}

// GetEntity returns the unique entity name linked to that service
func (s *KubeContainerService) GetEntity() string {
	return s.entity
}

// GetTaggerEntity returns the unique entity name linked to that service
func (s *KubeContainerService) GetTaggerEntity() string {
	taggerEntity, err := kubelet.KubeContainerIDToTaggerEntityID(s.entity)
	if err != nil {
		return s.entity
	}
	return taggerEntity
}

// GetADIdentifiers returns the service AD identifiers
func (s *KubeContainerService) GetADIdentifiers(context.Context) ([]string, error) {
	return s.adIdentifiers, nil
}

// GetHosts returns the pod hosts
func (s *KubeContainerService) GetHosts(context.Context) (map[string]string, error) {
	return s.hosts, nil
}

// GetPid is not supported for PodContainerService
func (s *KubeContainerService) GetPid(context.Context) (int, error) {
	return -1, ErrNotSupported
}

// GetPorts returns the container's ports
func (s *KubeContainerService) GetPorts(context.Context) ([]ContainerPort, error) {
	return s.ports, nil
}

// GetTags retrieves tags using the Tagger
func (s *KubeContainerService) GetTags() ([]string, string, error) {
	return tagger.TagWithHash(s.GetTaggerEntity(), tagger.ChecksCardinality)
}

// GetHostname returns nil and an error because port is not supported in Kubelet
func (s *KubeContainerService) GetHostname(context.Context) (string, error) {
	return "", ErrNotSupported
}

// GetCreationTime returns the creation time of the container compare to the agent start.
func (s *KubeContainerService) GetCreationTime() integration.CreationTime {
	return s.creationTime
}

// IsReady returns if the service is ready
func (s *KubeContainerService) IsReady(context.Context) bool {
	return s.ready
}

// GetExtraConfig resolves kubelet-specific template variables.
func (s *KubeContainerService) GetExtraConfig(key []byte) ([]byte, error) {
	result, found := s.extraConfig[string(key)]
	if !found {
		return []byte{}, fmt.Errorf("extra config %q is not supported", key)
	}

	return []byte(result), nil
}

// GetCheckNames returns names of checks defined in pod annotations
func (s *KubeContainerService) GetCheckNames(context.Context) []string {
	return s.checkNames
}

// HasFilter returns true if metrics or logs collection must be excluded for this service
// no containers.GlobalFilter case here because we don't create services that are globally excluded in AD
func (s *KubeContainerService) HasFilter(filter containers.FilterType) bool {
	switch filter {
	case containers.MetricsFilter:
		return s.metricsExcluded
	case containers.LogsFilter:
		return s.logsExcluded
	}
	return false
}

// GetEntity returns the unique entity name linked to that service
func (s *KubePodService) GetEntity() string {
	return s.entity
}

// GetTaggerEntity returns the unique entity name linked to that service
func (s *KubePodService) GetTaggerEntity() string {
	taggerEntity, err := kubelet.KubePodUIDToTaggerEntityID(s.entity)
	if err != nil {
		return s.entity
	}
	return taggerEntity
}

// GetADIdentifiers returns the service AD identifiers
func (s *KubePodService) GetADIdentifiers(context.Context) ([]string, error) {
	return s.adIdentifiers, nil
}

// GetHosts returns the pod hosts
func (s *KubePodService) GetHosts(context.Context) (map[string]string, error) {
	return s.hosts, nil
}

// GetPid is not supported for PodContainerService
func (s *KubePodService) GetPid(context.Context) (int, error) {
	return -1, ErrNotSupported
}

// GetPorts returns the container's ports
func (s *KubePodService) GetPorts(context.Context) ([]ContainerPort, error) {
	return s.ports, nil
}

// GetTags retrieves tags using the Tagger
func (s *KubePodService) GetTags() ([]string, string, error) {
	return tagger.TagWithHash(s.GetTaggerEntity(), tagger.ChecksCardinality)
}

// GetHostname returns nil and an error because port is not supported in Kubelet
func (s *KubePodService) GetHostname(context.Context) (string, error) {
	return "", ErrNotSupported
}

// GetCreationTime returns the creation time of the container compare to the agent start.
func (s *KubePodService) GetCreationTime() integration.CreationTime {
	return s.creationTime
}

// IsReady returns if the service is ready
func (s *KubePodService) IsReady(context.Context) bool {
	return true
}

// GetCheckNames returns slice of check names defined in kubernetes annotations or docker labels
// KubePodService doesn't implement this method
func (s *KubePodService) GetCheckNames(context.Context) []string {
	return nil
}

// HasFilter always return false
// KubePodService doesn't implement this method
func (s *KubePodService) HasFilter(filter containers.FilterType) bool {
	return false
}

// GetExtraConfig isn't supported
func (s *KubePodService) GetExtraConfig(key []byte) ([]byte, error) {
	return []byte{}, ErrNotSupported
}
