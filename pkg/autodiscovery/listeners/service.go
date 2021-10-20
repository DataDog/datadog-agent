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

func (s *service) GetADIdentifiers(_ context.Context) ([]string, error) {
	return s.adIdentifiers, nil
}

func (s *service) GetHosts(_ context.Context) (map[string]string, error) {
	return s.hosts, nil
}

func (s *service) GetPorts(_ context.Context) ([]ContainerPort, error) {
	return s.ports, nil
}

func (s *service) GetTags() ([]string, string, error) {
	return tagger.TagWithHash(s.GetTaggerEntity(), tagger.ChecksCardinality)
}

func (s *service) GetPid(_ context.Context) (int, error) {
	return s.pid, nil
}

func (s *service) GetHostname(_ context.Context) (string, error) {
	return s.hostname, nil
}

func (s *service) GetCreationTime() integration.CreationTime {
	return s.creationTime
}

func (s *service) IsReady(_ context.Context) bool {
	return s.ready
}

func (s *service) GetCheckNames(_ context.Context) []string {
	return s.checkNames
}

func (s *service) HasFilter(filter containers.FilterType) bool {
	switch filter {
	case containers.MetricsFilter:
		return s.metricsExcluded
	case containers.LogsFilter:
		return s.logsExcluded
	}

	return false
}

func (s *service) GetExtraConfig(key []byte) ([]byte, error) {
	result, found := s.extraConfig[string(key)]
	if !found {
		return []byte{}, fmt.Errorf("extra config %q is not supported", key)
	}

	return []byte(result), nil
}

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
