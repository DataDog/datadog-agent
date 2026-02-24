// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

//go:build kubelet && orchestrator && test

package kubeletconfig

import (
	"context"
	"strings"
	"testing"

	jsoniter "github.com/json-iterator/go"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.uber.org/fx"

	"github.com/DataDog/agent-payload/v5/process"

	apiv1 "github.com/DataDog/datadog-agent/pkg/clusteragent/api/v1"

	"github.com/DataDog/datadog-agent/comp/core"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	taggermock "github.com/DataDog/datadog-agent/comp/core/tagger/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/setup/constants"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	oconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/process"

	stypes "github.com/DataDog/datadog-agent/pkg/serializer/types"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/version"
)

var testHostName = "test-host"
var testNodeName = "node"
var staticRawKubeletConfig = []byte(`{"sample-key":"sample-value"}`)

type fakeSender struct {
	mocksender.MockSender
	manifests []process.MessageBody
}

type fakeDCAClient struct{}

func (f *fakeDCAClient) Version(_ bool) version.Version                    { panic("not used") }
func (f *fakeDCAClient) ClusterAgentAPIEndpoint() string                   { panic("not used") }
func (f *fakeDCAClient) GetNodeLabels(_ string) (map[string]string, error) { panic("not used") }
func (f *fakeDCAClient) GetNodeAnnotations(_ string, _ ...string) (map[string]string, error) {
	panic("not used")
}
func (f *fakeDCAClient) GetNodeUID(_ string) (string, error) {
	return "uid-test-123", nil
}
func (f *fakeDCAClient) GetNamespaceLabels(_ string) (map[string]string, error) {
	panic("not used")
}
func (f *fakeDCAClient) GetNamespaceMetadata(_ string) (*clusteragent.Metadata, error) {
	panic("not used")
}
func (f *fakeDCAClient) GetPodsMetadataForNode(_ string) (apiv1.NamespacesPodsStringsSet, error) {
	panic("not used")
}
func (f *fakeDCAClient) GetKubernetesMetadataNames(_, _, _ string) ([]string, error) {
	panic("not used")
}
func (f *fakeDCAClient) GetCFAppsMetadataForNode(_ string) (map[string][]string, error) {
	panic("not used")
}
func (f *fakeDCAClient) PostClusterCheckStatus(_ context.Context, _ string, _ types.NodeStatus) (types.StatusResponse, error) {
	panic("not used")
}
func (f *fakeDCAClient) GetClusterCheckConfigs(_ context.Context, _ string) (types.ConfigResponse, error) {
	panic("not used")
}
func (f *fakeDCAClient) GetEndpointsCheckConfigs(_ context.Context, _ string) (types.ConfigResponse, error) {
	panic("not used")
}
func (f *fakeDCAClient) GetKubernetesClusterID() (string, error) { panic("not used") }
func (f *fakeDCAClient) PostLanguageMetadata(_ context.Context, _ *pbgo.ParentLanguageAnnotationRequest) error {
	panic("not used")
}
func (f *fakeDCAClient) SupportsNamespaceMetadataCollection() bool { panic("not used") }

//nolint:revive // TODO(CAPP) Fix revive linter
func (s *fakeSender) OrchestratorManifest(msgs []stypes.ProcessMessageBody, clusterID string) {
	s.manifests = append(s.manifests, msgs...)
}

type KubeletConfigTestSuite struct {
	suite.Suite
	check  *Check
	sender *fakeSender
	tagger taggermock.Mock
}

func (suite *KubeletConfigTestSuite) SetupSuite() {
	kubelet.ResetGlobalKubeUtil()
	kubelet.ResetCache()
	jsoniter.RegisterTypeDecoder("kubelet.PodList", nil)
	mockConfig := configmock.New(suite.T())
	mockConfig.SetInTest("cluster_agent.enabled", true)
	mockConfig.SetInTest("kubernetes_kubelet_host", "127.0.0.1")
	mockConfig.SetInTest("kubelet_tls_verify", false)
	mockConfig.SetInTest("orchestrator_explorer.enabled", true)
	mockConfig.SetInTest("orchestrator_explorer.manifest_collection.enabled", true)
	mockConfig.SetInTest("kubernetes_pod_labels_as_tags", `{"tier":"dd_tier","component":"dd_component"}`)
	mockConfig.SetInTest("kubernetes_pod_annotations_as_tags", `{"kubernetes.io/config.source":"config_source","kubernetes.io/config.hash":"config_hash"}`)

	sender := &fakeSender{}
	suite.sender = sender

	mockStore := fxutil.Test[workloadmetamock.Mock](suite.T(), fx.Options(
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
	mockStore.Set(&workloadmeta.Kubelet{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubelet,
			ID:   workloadmeta.KubeletID,
		},
		RawConfig: staticRawKubeletConfig,
		NodeName:  testNodeName,
	})

	fakeTagger := taggerfxmock.SetupFakeTagger(suite.T())
	suite.tagger = fakeTagger

	suite.check = &Check{
		hostName: testHostName,
		sender:   sender,
		config:   oconfig.NewDefaultOrchestratorConfig(nil),
		cfg:      mockConfig,
		tagger:   fakeTagger,
		store:    mockStore,
	}

	getClusterAgentClient = func() (clusteragent.DCAClientInterface, error) {
		return &fakeDCAClient{}, nil
	}

}

func (suite *KubeletConfigTestSuite) TearDownSuite() {
}

func TestKubeletConfigTestSuite(t *testing.T) {
	suite.Run(t, new(KubeletConfigTestSuite))
}

func (suite *KubeletConfigTestSuite) TestKubeletConfigCheck() {
	cacheKey := cache.BuildAgentKey(constants.ClusterIDCacheKey)
	cachedClusterID, found := cache.Cache.Get(cacheKey)
	if !found {
		cache.Cache.Set(cacheKey, strings.Repeat("1", 36), cache.NoExpiration)
	}

	defer func() {
		cache.Cache.Set(cacheKey, cachedClusterID, cache.NoExpiration)
	}()

	err := suite.check.Run()
	require.NoError(suite.T(), err)
	require.Len(suite.T(), suite.sender.manifests, 1)
	manifestMsg, ok := suite.sender.manifests[0].(*process.CollectorManifest)
	manifest := manifestMsg.Manifests[0]

	require.True(suite.T(), ok)
	require.Equal(suite.T(), int32(orchestrator.K8sKubeletConfig), manifest.Type)
	require.Equal(suite.T(), suite.check.config.KubeClusterName, manifestMsg.ClusterName)
	require.Equal(suite.T(), suite.check.clusterID, manifestMsg.ClusterId)
	require.Equal(suite.T(), suite.check.hostName, manifestMsg.HostName)
	require.Len(suite.T(), manifestMsg.Manifests, 1)

	require.Equal(suite.T(), "application/json", manifest.ContentType)
	require.Equal(suite.T(), "v1", manifest.Version)
	require.False(suite.T(), manifest.IsTerminated)
	require.Equal(suite.T(), "KubeletConfiguration", manifest.Kind)
	require.Equal(suite.T(), "virtual.datadoghq.com/v1", manifest.ApiVersion)
	require.Equal(suite.T(), testNodeName, manifest.NodeName)
}
