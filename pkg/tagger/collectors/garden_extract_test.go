package collectors

import (
	"testing"

	"code.cloudfoundry.org/garden"
	"code.cloudfoundry.org/garden/gardenfakes"
	apiv1 "github.com/DataDog/datadog-agent/pkg/clusteragent/api/v1"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/stretchr/testify/assert"
)

type fakeDCAClient struct {
}

type fakeGardenUtil struct {
}

var expectedFullTags = map[string][]string{
	"app1": {
		"tag:1",
	},
	"app2": {
		"tag:2",
	},
}

var expectedIDTags = map[string][]string{
	"id1": {
		"container_name:id1",
		"app_instance_guid:id1",
	},
	"id2": {
		"container_name:id2",
		"app_instance_guid:id2",
	},
}

func (fakeGardenUtil) GetGardenContainers() ([]garden.Container, error) {
	return []garden.Container{
		&gardenfakes.FakeContainer{
			HandleStub: func() string {
				return "id1"
			},
		},
		&gardenfakes.FakeContainer{
			HandleStub: func() string {
				return "id2"
			},
		},
	}, nil
}

func (fakeDCAClient) GetCFAppsMetadataForNode(nodename string) (map[string][]string, error) {
	return expectedFullTags, nil
}

func TestGardenCollector_extractTags(t *testing.T) {
	collector := GardenCollector{
		gardenUtil:          fakeGardenUtil{},
		dcaClient:           fakeDCAClient{},
		clusterAgentEnabled: true,
	}
	tags, _ := collector.extractTags("cell123")
	assert.Equal(t, expectedFullTags, tags)

	collector.clusterAgentEnabled = false
	tags, _ = collector.extractTags("cell123")
	assert.Equal(t, expectedIDTags, tags)
}

// Unused DCAClientInterface methods
func (fakeDCAClient) Version() version.Version {
	panic("implement me")
}

func (fakeDCAClient) ClusterAgentAPIEndpoint() string {
	panic("implement me")
}

func (fakeDCAClient) GetVersion() (version.Version, error) {
	panic("implement me")
}

func (fakeDCAClient) GetNodeLabels(nodeName string) (map[string]string, error) {
	panic("implement me")
}

func (fakeDCAClient) GetPodsMetadataForNode(nodeName string) (apiv1.NamespacesPodsStringsSet, error) {
	panic("implement me")
}

func (fakeDCAClient) GetKubernetesMetadataNames(nodeName, ns, podName string) ([]string, error) {
	panic("implement me")
}

func (fakeDCAClient) PostClusterCheckStatus(nodeName string, status types.NodeStatus) (types.StatusResponse, error) {
	panic("implement me")
}

func (fakeDCAClient) GetClusterCheckConfigs(nodeName string) (types.ConfigResponse, error) {
	panic("implement me")
}

func (fakeDCAClient) GetEndpointsCheckConfigs(nodeName string) (types.ConfigResponse, error) {
	panic("implement me")
}

// Unused GardenUtilInterface methodes
func (fakeGardenUtil) ListContainers() ([]*containers.Container, error) {
	panic("implement me")
}

func (fakeGardenUtil) UpdateContainerMetrics(cList []*containers.Container) error {
	panic("implement me")
}
