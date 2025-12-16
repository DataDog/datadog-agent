// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator && test

package orchestrator

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.uber.org/fx"
	kscheme "k8s.io/client-go/kubernetes/scheme"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	crd "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	vpa "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/client/clientset/versioned/fake"
	cr "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors/inventory"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	mockconfig "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	orchcfg "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
)

func newCollectorBundle(t *testing.T, chk *OrchestratorCheck) *CollectorBundle {
	cfg := mockconfig.New(t)
	mockStore := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	fakeTagger := taggerfxmock.SetupFakeTagger(t)

	bundle := &CollectorBundle{
		discoverCollectors: chk.orchestratorConfig.CollectorDiscoveryEnabled,
		check:              chk,
		inventory:          inventory.NewCollectorInventory(cfg, mockStore, fakeTagger),
		runCfg: &collectors.CollectorRunConfig{
			K8sCollectorRunConfig: collectors.K8sCollectorRunConfig{
				APIClient:                   chk.apiClient,
				OrchestratorInformerFactory: chk.orchestratorInformerFactory,
			},
			ClusterID:   chk.clusterID,
			Config:      chk.orchestratorConfig,
			MsgGroupRef: chk.groupID,
		},
		stopCh:              chk.stopCh,
		manifestBuffer:      NewManifestBuffer(chk),
		activatedCollectors: map[string]struct{}{},
	}
	bundle.importCollectorsFromInventory()
	bundle.prepareExtraSyncTimeout()
	return bundle
}

// TestOrchestratorCheckSafeReSchedule close simulates the check being unscheduled and rescheduled again
func TestOrchestratorCheckSafeReSchedule(t *testing.T) {
	var wg sync.WaitGroup
	var scheme = kscheme.Scheme

	client := fake.NewSimpleClientset()
	vpaClient := vpa.NewSimpleClientset()
	crdClient := crd.NewSimpleClientset()
	crClient := cr.NewSimpleDynamicClient(scheme)
	cl := &apiserver.APIClient{InformerCl: client, VPAInformerClient: vpaClient, CRDInformerClient: crdClient, DynamicInformerCl: crClient}

	cfg := mockconfig.New(t)
	mockStore := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	fakeTagger := taggerfxmock.SetupFakeTagger(t)

	orchCheck := newCheck(cfg, mockStore, fakeTagger).(*OrchestratorCheck)
	mockSenderManager := mocksender.CreateDefaultDemultiplexer()
	_ = orchCheck.Configure(mockSenderManager, uint64(1), integration.Data{}, integration.Data{}, "test")
	orchCheck.apiClient = cl

	orchCheck.orchestratorInformerFactory = getOrchestratorInformerFactory(cl)
	bundle := newCollectorBundle(t, orchCheck)
	bundle.Initialize()

	wg.Add(2)

	// getting rescheduled.
	orchCheck.Cancel()

	bundle.runCfg.OrchestratorInformerFactory = getOrchestratorInformerFactory(cl)
	bundle.stopCh = make(chan struct{})
	bundle.initializeOnce = sync.Once{}
	bundle.Initialize()

	_, err := bundle.runCfg.OrchestratorInformerFactory.InformerFactory.Core().V1().Nodes().Informer().AddEventHandler(&cache.ResourceEventHandlerFuncs{
		AddFunc: func(_ interface{}) {
			wg.Done()
		},
	})
	assert.NoError(t, err)

	writeNode(t, client, "1")
	writeNode(t, client, "2")

	assert.True(t, waitTimeout(&wg, 2*time.Second))
}

func writeNode(t *testing.T, client *fake.Clientset, version string) {
	kubeN := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			ResourceVersion: version,
			UID:             types.UID("126430c6-5e57-11ea-91d5-42010a8400c6-" + version),
			Name:            "another-system-" + version,
		},
	}
	_, err := client.CoreV1().Nodes().Create(context.TODO(), &kubeN, metav1.CreateOptions{})
	assert.NoError(t, err)
}

// waitTimeout returns true if wg is completed and false if time is up
func waitTimeout(wg *sync.WaitGroup, timeout time.Duration) bool {
	c := make(chan struct{})
	go func() {
		defer close(c)
		wg.Wait()
	}()
	select {
	case <-c:
		return true
	case <-time.After(timeout):
		return false
	}
}

func TestOrchCheckExtraTags(t *testing.T) {

	cfg := mockconfig.New(t)
	mockStore := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
	mockSenderManager := mocksender.CreateDefaultDemultiplexer()

	t.Run("with no tags", func(t *testing.T) {
		fakeTagger := taggerfxmock.SetupFakeTagger(t)
		orchCheck := newCheck(cfg, mockStore, fakeTagger).(*OrchestratorCheck)

		_ = orchCheck.Configure(mockSenderManager, uint64(1), integration.Data{}, integration.Data{}, "test")
		assert.Empty(t, orchCheck.orchestratorConfig.ExtraTags)
	})

	t.Run("with tagger tags", func(t *testing.T) {
		fakeTagger := taggerfxmock.SetupFakeTagger(t)
		fakeTagger.SetGlobalTags([]string{"tag1:value1", "tag2:value2"}, nil, nil, nil)
		orchCheck := newCheck(cfg, mockStore, fakeTagger).(*OrchestratorCheck)

		_ = orchCheck.Configure(mockSenderManager, uint64(1), integration.Data{}, integration.Data{}, "test")
		assert.ElementsMatch(t, []string{"tag1:value1", "tag2:value2"}, orchCheck.orchestratorConfig.ExtraTags)
	})

	t.Run("with check tags", func(t *testing.T) {
		fakeTagger := taggerfxmock.SetupFakeTagger(t)
		orchCheck := newCheck(cfg, mockStore, fakeTagger).(*OrchestratorCheck)

		initConfigData := integration.Data(`{}`)
		instanceConfigData := integration.Data(`{}`)

		err := initConfigData.MergeAdditionalTags([]string{"init_tag1:value1", "init_tag2:value2"})
		assert.NoError(t, err)

		err = instanceConfigData.MergeAdditionalTags([]string{"instance_tag1:value1", "instance_tag2:value2"})
		assert.NoError(t, err)

		_ = orchCheck.Configure(mockSenderManager, uint64(1), initConfigData, instanceConfigData, "test")
		assert.ElementsMatch(t, []string{"init_tag1:value1", "init_tag2:value2", "instance_tag1:value1", "instance_tag2:value2"}, orchCheck.orchestratorConfig.ExtraTags)
	})

}

func TestOrchestratorCheckConfigure(t *testing.T) {
	cfg := mockconfig.New(t)
	mockStore := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
	mockSenderManager := mocksender.CreateDefaultDemultiplexer()

	setupMockAPIClient := func(orchCheck *OrchestratorCheck) {
		client := fake.NewSimpleClientset()
		vpaClient := vpa.NewSimpleClientset()
		crdClient := crd.NewSimpleClientset()
		orchCheck.apiClient = &apiserver.APIClient{
			InformerCl:        client,
			VPAInformerClient: vpaClient,
			CRDInformerClient: crdClient,
		}
	}

	// Helper function to setup test configuration
	setupGlobalConfig := func() {
		// Enable Kubernetes feature for cluster name detection
		env.SetFeatures(t, env.Kubernetes)
		// Reset cluster name before each test
		clustername.ResetClusterName()
		// Set configuration in global Datadog config
		pkgconfigsetup.Datadog().SetWithoutSource("orchestrator_explorer.enabled", true)
		pkgconfigsetup.Datadog().SetWithoutSource("cluster_name", "test-cluster")
		// Set cluster ID environment variable to avoid cluster agent calls
		t.Setenv("DD_ORCHESTRATOR_CLUSTER_ID", "d801b2b1-4811-11ea-8618-121d4d0938a3")
	}

	t.Run("failure when orchestrator collection is disabled", func(t *testing.T) {
		env.SetFeatures(t, env.Kubernetes)
		clustername.ResetClusterName()
		pkgconfigsetup.Datadog().SetWithoutSource("orchestrator_explorer.enabled", false)
		t.Setenv("DD_ORCHESTRATOR_CLUSTER_ID", "d801b2b1-4811-11ea-8618-121d4d0938a3")

		fakeTagger := taggerfxmock.SetupFakeTagger(t)
		orchCheck := newCheck(cfg, mockStore, fakeTagger).(*OrchestratorCheck)
		setupMockAPIClient(orchCheck)

		err := orchCheck.Configure(mockSenderManager, uint64(1), integration.Data{}, integration.Data{}, "test")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "orchestrator check is configured but the feature is disabled")
	})

	t.Run("failure when cluster name is empty", func(t *testing.T) {
		env.SetFeatures(t, env.Kubernetes)
		clustername.ResetClusterName()
		pkgconfigsetup.Datadog().SetWithoutSource("orchestrator_explorer.enabled", true)
		// Don't set cluster_name to test empty cluster name scenario
		t.Setenv("DD_ORCHESTRATOR_CLUSTER_ID", "d801b2b1-4811-11ea-8618-121d4d0938a3")

		fakeTagger := taggerfxmock.SetupFakeTagger(t)
		orchCheck := newCheck(cfg, mockStore, fakeTagger).(*OrchestratorCheck)
		setupMockAPIClient(orchCheck)

		err := orchCheck.Configure(mockSenderManager, uint64(1), integration.Data{}, integration.Data{}, "test")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "orchestrator check is configured but the cluster name is empty")
	})

	t.Run("orchestrator config loads correctly", func(t *testing.T) {
		setupGlobalConfig()

		fakeTagger := taggerfxmock.SetupFakeTagger(t)
		orchCheck := newCheck(cfg, mockStore, fakeTagger).(*OrchestratorCheck)

		// Test orchestrator config loading separately from Configure
		orchCheck.orchestratorConfig = orchcfg.NewDefaultOrchestratorConfig([]string{"env:test"})
		err := orchCheck.orchestratorConfig.Load()
		assert.NoError(t, err)

		// Verify cluster name is loaded correctly
		assert.True(t, orchCheck.orchestratorConfig.OrchestrationCollectionEnabled)
		assert.Equal(t, "test-cluster", orchCheck.orchestratorConfig.KubeClusterName)

		// Test cluster ID loading
		clusterID, err := clustername.GetClusterID()
		assert.NoError(t, err)
		assert.Equal(t, "d801b2b1-4811-11ea-8618-121d4d0938a3", clusterID)
	})

	t.Run("instance configuration parsing works correctly", func(t *testing.T) {
		setupGlobalConfig()

		fakeTagger := taggerfxmock.SetupFakeTagger(t)
		orchCheck := newCheck(cfg, mockStore, fakeTagger).(*OrchestratorCheck)

		instanceConfigData := integration.Data(`
collectors:
  - nodes
  - pods
crd_collectors:
  - datadoghq.com/v1alpha1/datadogmetrics
extra_sync_timeout_seconds: 30
`)

		// Test instance parsing separately
		err := orchCheck.instance.parse(instanceConfigData)
		assert.NoError(t, err)

		// Verify instance configuration was parsed correctly
		assert.Equal(t, []string{"nodes", "pods"}, orchCheck.instance.Collectors)
		assert.Equal(t, []string{"datadoghq.com/v1alpha1/datadogmetrics"}, orchCheck.instance.CRDCollectors)
		assert.Equal(t, 30, orchCheck.instance.ExtraSyncTimeoutSeconds)
	})

	t.Run("configuration loading with custom settings", func(t *testing.T) {
		env.SetFeatures(t, env.Kubernetes)
		clustername.ResetClusterName()

		// Set custom configuration values
		pkgconfigsetup.Datadog().SetWithoutSource("orchestrator_explorer.enabled", true)
		pkgconfigsetup.Datadog().SetWithoutSource("cluster_name", "custom-cluster")
		pkgconfigsetup.Datadog().SetWithoutSource("orchestrator_explorer.max_per_message", 75)
		pkgconfigsetup.Datadog().SetWithoutSource("orchestrator_explorer.max_message_bytes", 30000000)
		pkgconfigsetup.Datadog().SetWithoutSource("orchestrator_explorer.collector_discovery.enabled", true)
		pkgconfigsetup.Datadog().SetWithoutSource("orchestrator_explorer.container_scrubbing.enabled", true)
		pkgconfigsetup.Datadog().SetWithoutSource("orchestrator_explorer.manifest_collection.enabled", true)
		t.Setenv("DD_ORCHESTRATOR_CLUSTER_ID", "d801b2b1-4811-11ea-8618-121d4d0938a3")

		fakeTagger := taggerfxmock.SetupFakeTagger(t)
		orchCheck := newCheck(cfg, mockStore, fakeTagger).(*OrchestratorCheck)

		// Test orchestrator config loading with custom settings
		orchCheck.orchestratorConfig = orchcfg.NewDefaultOrchestratorConfig([]string{"custom:tag"})
		err := orchCheck.orchestratorConfig.Load()
		assert.NoError(t, err)

		// Verify custom configuration was loaded
		assert.True(t, orchCheck.orchestratorConfig.OrchestrationCollectionEnabled)
		assert.Equal(t, "custom-cluster", orchCheck.orchestratorConfig.KubeClusterName)
		assert.Equal(t, 75, orchCheck.orchestratorConfig.MaxPerMessage)
		assert.Equal(t, 30000000, orchCheck.orchestratorConfig.MaxWeightPerMessageBytes)
		assert.True(t, orchCheck.orchestratorConfig.CollectorDiscoveryEnabled)
		assert.True(t, orchCheck.orchestratorConfig.IsScrubbingEnabled)
		assert.True(t, orchCheck.orchestratorConfig.IsManifestCollectionEnabled)
		assert.Equal(t, []string{"custom:tag"}, orchCheck.orchestratorConfig.ExtraTags)
	})
}

func TestOrchestratorCheck_IsLeader(t *testing.T) {
	cfg := mockconfig.New(t)
	mockStore := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	t.Run("returns true when CLC runner with LeaderSkip enabled", func(t *testing.T) {
		fakeTagger := taggerfxmock.SetupFakeTagger(t)
		orchCheck := newCheck(cfg, mockStore, fakeTagger).(*OrchestratorCheck)

		// Configure as CLC runner with LeaderSkip
		orchCheck.isCLCRunner = true
		orchCheck.instance = &OrchestratorInstance{
			LeaderSkip: true,
		}

		isLeader, err := orchCheck.IsLeader()
		assert.True(t, isLeader)
		assert.NoError(t, err)
	})

	t.Run("checks leader election when CLC runner but LeaderSkip disabled", func(t *testing.T) {
		fakeTagger := taggerfxmock.SetupFakeTagger(t)
		orchCheck := newCheck(cfg, mockStore, fakeTagger).(*OrchestratorCheck)

		// Configure as CLC runner but without LeaderSkip
		orchCheck.isCLCRunner = true
		orchCheck.instance = &OrchestratorInstance{
			LeaderSkip: false,
		}

		// Leader election is not enabled
		pkgconfigsetup.Datadog().SetWithoutSource("leader_election", false)

		isLeader, err := orchCheck.IsLeader()
		assert.False(t, isLeader)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Leader Election not enabled")
	})

	t.Run("checks leader election when not CLC runner", func(t *testing.T) {
		fakeTagger := taggerfxmock.SetupFakeTagger(t)
		orchCheck := newCheck(cfg, mockStore, fakeTagger).(*OrchestratorCheck)

		// Configure as non-CLC runner
		orchCheck.isCLCRunner = false
		orchCheck.instance = &OrchestratorInstance{
			LeaderSkip: false,
		}

		// Leader election is not enabled
		pkgconfigsetup.Datadog().SetWithoutSource("leader_election", false)

		isLeader, err := orchCheck.IsLeader()
		assert.False(t, isLeader)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Leader Election not enabled")
	})

	t.Run("returns error when leader election not enabled", func(t *testing.T) {
		fakeTagger := taggerfxmock.SetupFakeTagger(t)
		orchCheck := newCheck(cfg, mockStore, fakeTagger).(*OrchestratorCheck)

		orchCheck.isCLCRunner = false
		orchCheck.instance = &OrchestratorInstance{}

		// Explicitly disable leader election
		pkgconfigsetup.Datadog().SetWithoutSource("leader_election", false)

		isLeader, err := orchCheck.IsLeader()
		assert.False(t, isLeader)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Leader Election not enabled")
	})

	t.Run("skips leader election when both CLC runner and LeaderSkip are true", func(t *testing.T) {
		fakeTagger := taggerfxmock.SetupFakeTagger(t)
		orchCheck := newCheck(cfg, mockStore, fakeTagger).(*OrchestratorCheck)

		orchCheck.isCLCRunner = true
		orchCheck.instance = &OrchestratorInstance{
			LeaderSkip: true,
		}

		// Even if leader election is disabled, should still return true
		pkgconfigsetup.Datadog().SetWithoutSource("leader_election", false)

		isLeader, err := orchCheck.IsLeader()
		assert.True(t, isLeader)
		assert.NoError(t, err)
	})
}
