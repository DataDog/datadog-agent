package listeners

import (
	"context"
	"fmt"
	"reflect"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

// service implements the Service interface and stores data collected from
// workloadmeta.Store.
type service struct {
	entity          workloadmeta.Entity
	adIdentifiers   []string
	hosts           map[string]string
	ports           []ContainerPort
	pid             int
	hostname        string
	creationTime    integration.CreationTime
	ready           bool
	checkNames      []string
	extraConfig     map[string]string
	metricsExcluded bool
	logsExcluded    bool
}

var _ Service = &service{}

// GetEntity returns the AD entity ID of the service.
func (s *service) GetEntity() string {
	switch e := s.entity.(type) {
	case *workloadmeta.Container:
		return containers.BuildEntityName(string(e.Runtime), e.ID)
	case *workloadmeta.KubernetesPod:
		return kubelet.PodUIDToEntityName(e.ID)
	default:
		entityID := s.entity.GetID()
		log.Errorf("cannot build AD entity ID for kind %q, ID %q", entityID.Kind, entityID.ID)
		return ""
	}
}

// GetTaggerEntity returns the Tagger entity ID of the service.
func (s *service) GetTaggerEntity() string {
	switch e := s.entity.(type) {
	case *workloadmeta.Container:
		return containers.BuildTaggerEntityName(e.ID)
	case *workloadmeta.KubernetesPod:
		return kubelet.PodUIDToTaggerEntityName(e.ID)
	default:
		entityID := s.entity.GetID()
		log.Errorf("cannot build AD entity ID for kind %q, ID %q", entityID.Kind, entityID.ID)
		return ""
	}
}

// GetADIdentifiers returns the service's AD identifiers.
func (s *service) GetADIdentifiers(_ context.Context) ([]string, error) {
	return s.adIdentifiers, nil
}

// GetHosts returns the service's IPs for each host.
func (s *service) GetHosts(_ context.Context) (map[string]string, error) {
	return s.hosts, nil
}

// GetPorts returns the ports exposed by the service's containers.
func (s *service) GetPorts(_ context.Context) ([]ContainerPort, error) {
	return s.ports, nil
}

// GetTags returns the tags associated with the service.
func (s *service) GetTags() ([]string, string, error) {
	return tagger.TagWithHash(s.GetTaggerEntity(), tagger.ChecksCardinality)
}

// GetPid returns the process ID of the service.
func (s *service) GetPid(_ context.Context) (int, error) {
	return s.pid, nil
}

// GetHostname returns the service's hostname.
func (s *service) GetHostname(_ context.Context) (string, error) {
	return s.hostname, nil
}

// GetCreationTime returns whether the service was created before or after the
// first run of the collector that created it.
func (s *service) GetCreationTime() integration.CreationTime {
	return s.creationTime
}

// IsReady returns whether the service is ready.
func (s *service) IsReady(_ context.Context) bool {
	return s.ready
}

// GetCheckNames returns the check names of the service.
func (s *service) GetCheckNames(_ context.Context) []string {
	return s.checkNames
}

// HasFilter returns whether the service should not collect certain data (logs
// or metrics) due to filtering applied by filter.
func (s *service) HasFilter(filter containers.FilterType) bool {
	switch filter {
	case containers.MetricsFilter:
		return s.metricsExcluded
	case containers.LogsFilter:
		return s.logsExcluded
	}

	return false
}

// GetExtraConfig returns extra configuration associated with the service.
func (s *service) GetExtraConfig(key []byte) ([]byte, error) {
	result, found := s.extraConfig[string(key)]
	if !found {
		return []byte{}, fmt.Errorf("extra config %q is not supported", key)
	}

	return []byte(result), nil
}

// svcEqual checks that two Services are equal to each other by doing a deep
// equality check on data returned by most of Service's methods. Methods not
// checked are HasFilter and GetExtraConfig.
func svcEqual(a, b Service) bool {
	ctx := context.Background()

	var (
		errA error
		errB error
	)

	entityA := a.GetEntity()
	entityB := b.GetEntity()
	if entityA != entityB {
		return false
	}

	hostsA, errA := a.GetHosts(ctx)
	hostsB, errB := b.GetHosts(ctx)
	if errA != errB || !reflect.DeepEqual(hostsA, hostsB) {
		return false
	}

	portsA, errA := a.GetPorts(ctx)
	portsB, errB := b.GetPorts(ctx)
	if errA != errB && !reflect.DeepEqual(portsA, portsB) {
		return false
	}

	adA, errA := a.GetADIdentifiers(ctx)
	adB, errB := b.GetADIdentifiers(ctx)
	if errA != errB || !reflect.DeepEqual(adA, adB) {
		return false
	}

	if !reflect.DeepEqual(a.GetCheckNames(ctx), b.GetCheckNames(ctx)) {
		return false
	}

	hostnameA, errA := a.GetHostname(ctx)
	hostnameB, errB := b.GetHostname(ctx)
	if errA != errB || hostnameA != hostnameB {
		return false
	}

	pidA, errA := a.GetPid(ctx)
	pidB, errB := b.GetPid(ctx)
	if errA != errB || pidA != pidB {
		return false
	}

	return a.GetCreationTime() == b.GetCreationTime() &&
		a.IsReady(ctx) == b.IsReady(ctx)
}
