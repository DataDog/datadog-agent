package cloudfoundry_vm

import (
	"context"

	apiv1 "github.com/DataDog/datadog-agent/pkg/clusteragent/api/v1"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/process"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// type collector struct {
// 	store workloadmeta.Store
// 	seen  map[workloadmeta.EntityID]struct{}

// 	gardenUtil cloudfoundry.GardenUtilInterface
// 	nodeName   string

// 	dcaClient  clusteragent.DCAClientInterface
// 	dcaEnabled bool
// }

type FakeDCAClient struct {
	LocalVersion                 version.Version
	LocalClusterAgentAPIEndpoint string

	VersionErr error

	NodeLabels    map[string]string
	NodeLabelsErr error

	NodeAnnotations    map[string]string
	NodeAnnotationsErr error

	NamespaceLabels    map[string]string
	NamespaceLabelsErr error

	PodMetadataForNode    apiv1.NamespacesPodsStringsSet
	PodMetadataForNodeErr error

	KubernetesMetadataNames    []string
	KubernetesMetadataNamesErr error

	ClusterCheckStatus    types.StatusResponse
	ClusterCheckStatusErr error

	ClusterCheckConfigs    types.ConfigResponse
	ClusterCheckConfigsErr error

	EndpointsCheckConfigs    types.ConfigResponse
	EndpointsCheckConfigsErr error

	ClusterID    string
	ClusterIDErr error
}

func (f *FakeDCAClient) Version() version.Version {
	panic("implement me")
}

func (f *FakeDCAClient) ClusterAgentAPIEndpoint() string {
	panic("implement me")
}

func (f *FakeDCAClient) GetVersion() (version.Version, error) {
	panic("implement me")
}

func (f *FakeDCAClient) GetNodeLabels(nodeName string) (map[string]string, error) {
	panic("implement me")
}

func (f *FakeDCAClient) GetNodeAnnotations(nodeName string) (map[string]string, error) {
	panic("implement me")
}

func (f *FakeDCAClient) GetNamespaceLabels(nsName string) (map[string]string, error) {
	panic("implement me")
}

func (f *FakeDCAClient) GetPodsMetadataForNode(nodeName string) (apiv1.NamespacesPodsStringsSet, error) {
	panic("implement me")
}

func (f *FakeDCAClient) GetKubernetesMetadataNames(nodeName, ns, podName string) ([]string, error) {
	panic("implement me")
}

func (f *FakeDCAClient) PostClusterCheckStatus(ctx context.Context, identifier string, status types.NodeStatus) (types.StatusResponse, error) {
	panic("implement me")
}

func (f *FakeDCAClient) GetClusterCheckConfigs(ctx context.Context, identifier string) (types.ConfigResponse, error) {
	panic("implement me")
}

func (f *FakeDCAClient) GetEndpointsCheckConfigs(ctx context.Context, nodeName string) (types.ConfigResponse, error) {
	panic("implement me")
}

func (f *FakeDCAClient) GetKubernetesClusterID() (string, error) {
	panic("implement me")
}

func (f *FakeDCAClient) GetCFAppsMetadataForNode(_ string) (map[string][]string, error) {
	panic("implement me")
}

func (f *FakeDCAClient) PostLanguageMetadata(_ context.Context, _ *pbgo.ParentLanguageAnnotationRequest) error {
	panic("implement me")
}
