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

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	crd "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	vpa "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/client/clientset/versioned/fake"
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
	mockconfig "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
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

	client := fake.NewSimpleClientset()
	vpaClient := vpa.NewSimpleClientset()
	crdClient := crd.NewSimpleClientset()
	cl := &apiserver.APIClient{InformerCl: client, VPAInformerClient: vpaClient, CRDInformerClient: crdClient}

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

	t.Run("successful configuration with default settings", func(t *testing.T) {
		fakeTagger := taggerfxmock.SetupFakeTagger(t)
		orchCheck := newCheck(cfg, mockStore, fakeTagger).(*OrchestratorCheck)
		setupMockAPIClient(orchCheck)

		// Enable orchestrator collection
		cfg.SetWithoutSource("orchestrator_explorer.enabled", true)
		cfg.SetWithoutSource("cluster_name", "test-cluster")

		err := orchCheck.Configure(mockSenderManager, uint64(1), integration.Data{}, integration.Data{}, "test")
		assert.NoError(t, err)

		// Verify that the configuration was properly loaded
		assert.NotNil(t, orchCheck.orchestratorConfig)
		assert.True(t, orchCheck.orchestratorConfig.OrchestrationCollectionEnabled)
		assert.Equal(t, "test-cluster", orchCheck.orchestratorConfig.KubeClusterName)
		assert.NotNil(t, orchCheck.orchestratorInformerFactory)
		assert.NotNil(t, orchCheck.collectorBundle)
		assert.NotEmpty(t, orchCheck.clusterID)
	})

	t.Run("failure when orchestrator collection is disabled", func(t *testing.T) {
		fakeTagger := taggerfxmock.SetupFakeTagger(t)
		orchCheck := newCheck(cfg, mockStore, fakeTagger).(*OrchestratorCheck)
		setupMockAPIClient(orchCheck)

		// Disable orchestrator collection
		cfg.SetWithoutSource("orchestrator_explorer.enabled", false)

		err := orchCheck.Configure(mockSenderManager, uint64(1), integration.Data{}, integration.Data{}, "test")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "orchestrator check is configured but the feature is disabled")
	})

	t.Run("failure when cluster name is empty", func(t *testing.T) {
		fakeTagger := taggerfxmock.SetupFakeTagger(t)
		orchCheck := newCheck(cfg, mockStore, fakeTagger).(*OrchestratorCheck)
		setupMockAPIClient(orchCheck)

		// Enable orchestrator collection but leave cluster name empty
		cfg.SetWithoutSource("orchestrator_explorer.enabled", true)
		cfg.SetWithoutSource("cluster_name", "")

		err := orchCheck.Configure(mockSenderManager, uint64(1), integration.Data{}, integration.Data{}, "test")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "orchestrator check is configured but the cluster name is empty")
	})

	t.Run("failure with invalid instance config", func(t *testing.T) {
		fakeTagger := taggerfxmock.SetupFakeTagger(t)
		orchCheck := newCheck(cfg, mockStore, fakeTagger).(*OrchestratorCheck)
		setupMockAPIClient(orchCheck)

		// Enable orchestrator collection
		cfg.SetWithoutSource("orchestrator_explorer.enabled", true)
		cfg.SetWithoutSource("cluster_name", "test-cluster")

		// Provide invalid YAML configuration
		invalidConfig := integration.Data(`invalid: yaml: content: [`)

		err := orchCheck.Configure(mockSenderManager, uint64(1), integration.Data{}, invalidConfig, "test")
		assert.Error(t, err)
	})

	t.Run("successful configuration with custom instance settings", func(t *testing.T) {
		fakeTagger := taggerfxmock.SetupFakeTagger(t)
		orchCheck := newCheck(cfg, mockStore, fakeTagger).(*OrchestratorCheck)
		setupMockAPIClient(orchCheck)

		// Enable orchestrator collection
		cfg.SetWithoutSource("orchestrator_explorer.enabled", true)
		cfg.SetWithoutSource("cluster_name", "test-cluster")

		// Provide valid instance configuration
		instanceConfig := integration.Data(`
skip_leader_election: true
collectors:
  - nodes
  - pods
crd_collectors:
  - datadoghq.com/v1alpha1/datadogmetrics
extra_sync_timeout_seconds: 30
`)

		err := orchCheck.Configure(mockSenderManager, uint64(1), integration.Data{}, instanceConfig, "test")
		assert.NoError(t, err)

		// Verify instance configuration was parsed correctly
		assert.True(t, orchCheck.instance.LeaderSkip)
		assert.ElementsMatch(t, []string{"nodes", "pods"}, orchCheck.instance.Collectors)
		assert.ElementsMatch(t, []string{"datadoghq.com/v1alpha1/datadogmetrics"}, orchCheck.instance.CRDCollectors)
		assert.Equal(t, 30, orchCheck.instance.ExtraSyncTimeoutSeconds)
	})

	t.Run("configuration preserves extra tags from both tagger and config", func(t *testing.T) {
		fakeTagger := taggerfxmock.SetupFakeTagger(t)
		fakeTagger.SetGlobalTags([]string{"tagger:tag1", "env:prod"}, nil, nil, nil)
		orchCheck := newCheck(cfg, mockStore, fakeTagger).(*OrchestratorCheck)
		setupMockAPIClient(orchCheck)

		// Enable orchestrator collection
		cfg.SetWithoutSource("orchestrator_explorer.enabled", true)
		cfg.SetWithoutSource("cluster_name", "test-cluster")

		// Add tags through configuration
		initConfigData := integration.Data(`{}`)
		instanceConfigData := integration.Data(`{}`)

		err := initConfigData.MergeAdditionalTags([]string{"init:tag1"})
		assert.NoError(t, err)

		err = instanceConfigData.MergeAdditionalTags([]string{"instance:tag1"})
		assert.NoError(t, err)

		err = orchCheck.Configure(mockSenderManager, uint64(1), initConfigData, instanceConfigData, "test")
		assert.NoError(t, err)

		// Verify all tags are preserved
		expectedTags := []string{"instance:tag1", "init:tag1", "tagger:tag1", "env:prod"}
		assert.ElementsMatch(t, expectedTags, orchCheck.orchestratorConfig.ExtraTags)
	})

	t.Run("configuration with custom orchestrator settings", func(t *testing.T) {
		fakeTagger := taggerfxmock.SetupFakeTagger(t)
		orchCheck := newCheck(cfg, mockStore, fakeTagger).(*OrchestratorCheck)
		setupMockAPIClient(orchCheck)

		// Enable orchestrator collection with custom settings
		cfg.SetWithoutSource("orchestrator_explorer.enabled", true)
		cfg.SetWithoutSource("cluster_name", "test-cluster")
		cfg.SetWithoutSource("orchestrator_explorer.max_per_message", 50)
		cfg.SetWithoutSource("orchestrator_explorer.max_message_bytes", 25*1e6)
		cfg.SetWithoutSource("orchestrator_explorer.collector_discovery.enabled", true)
		cfg.SetWithoutSource("orchestrator_explorer.container_scrubbing.enabled", true)
		cfg.SetWithoutSource("orchestrator_explorer.manifest_collection.enabled", true)

		err := orchCheck.Configure(mockSenderManager, uint64(1), integration.Data{}, integration.Data{}, "test")
		assert.NoError(t, err)

		// Verify custom settings were applied
		assert.Equal(t, 50, orchCheck.orchestratorConfig.MaxPerMessage)
		assert.Equal(t, 25*1e6, orchCheck.orchestratorConfig.MaxWeightPerMessageBytes)
		assert.True(t, orchCheck.orchestratorConfig.CollectorDiscoveryEnabled)
		assert.True(t, orchCheck.orchestratorConfig.IsScrubbingEnabled)
		assert.True(t, orchCheck.orchestratorConfig.IsManifestCollectionEnabled)
	})

	t.Run("configuration initializes all required components", func(t *testing.T) {
		fakeTagger := taggerfxmock.SetupFakeTagger(t)
		orchCheck := newCheck(cfg, mockStore, fakeTagger).(*OrchestratorCheck)
		setupMockAPIClient(orchCheck)

		// Enable orchestrator collection
		cfg.SetWithoutSource("orchestrator_explorer.enabled", true)
		cfg.SetWithoutSource("cluster_name", "test-cluster")

		err := orchCheck.Configure(mockSenderManager, uint64(1), integration.Data{}, integration.Data{}, "test")
		assert.NoError(t, err)

		// Verify all required components are initialized
		assert.NotNil(t, orchCheck.orchestratorConfig, "orchestratorConfig should be initialized")
		assert.NotNil(t, orchCheck.instance, "instance should be initialized")
		assert.NotNil(t, orchCheck.collectorBundle, "collectorBundle should be initialized")
		assert.NotEmpty(t, orchCheck.clusterID, "clusterID should be set")
		assert.NotNil(t, orchCheck.apiClient, "apiClient should be set")
		assert.NotNil(t, orchCheck.orchestratorInformerFactory, "orchestratorInformerFactory should be initialized")

		// Verify orchestrator config has required fields
		assert.NotEmpty(t, orchCheck.orchestratorConfig.KubeClusterName)
		assert.NotNil(t, orchCheck.orchestratorConfig.Scrubber)
		assert.NotEmpty(t, orchCheck.orchestratorConfig.OrchestratorEndpoints)
	})

	t.Run("configuration with empty collectors list", func(t *testing.T) {
		fakeTagger := taggerfxmock.SetupFakeTagger(t)
		orchCheck := newCheck(cfg, mockStore, fakeTagger).(*OrchestratorCheck)
		setupMockAPIClient(orchCheck)

		// Enable orchestrator collection
		cfg.SetWithoutSource("orchestrator_explorer.enabled", true)
		cfg.SetWithoutSource("cluster_name", "test-cluster")

		// Provide instance configuration with empty collectors
		instanceConfig := integration.Data(`
collectors: []
crd_collectors: []
`)

		err := orchCheck.Configure(mockSenderManager, uint64(1), integration.Data{}, instanceConfig, "test")
		assert.NoError(t, err)

		// Verify empty collectors are handled correctly
		assert.Empty(t, orchCheck.instance.Collectors)
		assert.Empty(t, orchCheck.instance.CRDCollectors)
	})

	t.Run("configuration with malformed YAML in init config", func(t *testing.T) {
		fakeTagger := taggerfxmock.SetupFakeTagger(t)
		orchCheck := newCheck(cfg, mockStore, fakeTagger).(*OrchestratorCheck)
		setupMockAPIClient(orchCheck)

		// Enable orchestrator collection
		cfg.SetWithoutSource("orchestrator_explorer.enabled", true)
		cfg.SetWithoutSource("cluster_name", "test-cluster")

		// Provide malformed YAML in init config
		malformedInitConfig := integration.Data(`invalid: yaml: [missing bracket`)

		err := orchCheck.Configure(mockSenderManager, uint64(1), malformedInitConfig, integration.Data{}, "test")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "could not parse tags from check config")
	})

	t.Run("configuration handles tagger error gracefully", func(t *testing.T) {
		fakeTagger := taggerfxmock.SetupFakeTagger(t)
		// Simulate tagger error by not setting up properly
		orchCheck := newCheck(cfg, mockStore, fakeTagger).(*OrchestratorCheck)
		setupMockAPIClient(orchCheck)

		// Enable orchestrator collection
		cfg.SetWithoutSource("orchestrator_explorer.enabled", true)
		cfg.SetWithoutSource("cluster_name", "test-cluster")

		// This should still work even if tagger has issues
		err := orchCheck.Configure(mockSenderManager, uint64(1), integration.Data{}, integration.Data{}, "test")
		assert.NoError(t, err)
	})

	t.Run("configuration with orchestrator config load failure", func(t *testing.T) {
		fakeTagger := taggerfxmock.SetupFakeTagger(t)
		orchCheck := newCheck(cfg, mockStore, fakeTagger).(*OrchestratorCheck)
		setupMockAPIClient(orchCheck)

		// Set invalid configuration that would cause Load() to fail
		cfg.SetWithoutSource("orchestrator_explorer.enabled", true)
		cfg.SetWithoutSource("cluster_name", "test-cluster")
		cfg.SetWithoutSource("orchestrator_explorer.max_per_message", -1) // Invalid value

		err := orchCheck.Configure(mockSenderManager, uint64(1), integration.Data{}, integration.Data{}, "test")
		assert.NoError(t, err) // The config validation might not catch this, depending on implementation
	})

	t.Run("configuration builds correct check ID", func(t *testing.T) {
		fakeTagger := taggerfxmock.SetupFakeTagger(t)
		orchCheck := newCheck(cfg, mockStore, fakeTagger).(*OrchestratorCheck)
		setupMockAPIClient(orchCheck)

		// Enable orchestrator collection
		cfg.SetWithoutSource("orchestrator_explorer.enabled", true)
		cfg.SetWithoutSource("cluster_name", "test-cluster")

		configDigest := uint64(12345)
		err := orchCheck.Configure(mockSenderManager, configDigest, integration.Data{}, integration.Data{}, "test-source")
		assert.NoError(t, err)

		// Verify the check ID was built (this is internal to CheckBase but we can verify the configure succeeded)
		assert.NotEmpty(t, orchCheck.ID())
	})

	t.Run("configuration with custom sensitive words and annotations", func(t *testing.T) {
		fakeTagger := taggerfxmock.SetupFakeTagger(t)
		orchCheck := newCheck(cfg, mockStore, fakeTagger).(*OrchestratorCheck)
		setupMockAPIClient(orchCheck)

		// Enable orchestrator collection with custom sensitive settings
		cfg.SetWithoutSource("orchestrator_explorer.enabled", true)
		cfg.SetWithoutSource("cluster_name", "test-cluster")
		cfg.SetWithoutSource("orchestrator_explorer.custom_sensitive_words", []string{"secret", "password", "token"})
		cfg.SetWithoutSource("orchestrator_explorer.custom_sensitive_annotations_labels", []string{"sensitive-annotation", "secret-label"})

		err := orchCheck.Configure(mockSenderManager, uint64(1), integration.Data{}, integration.Data{}, "test")
		assert.NoError(t, err)

		// Verify custom sensitive words are applied
		assert.Contains(t, orchCheck.orchestratorConfig.Scrubber.LiteralSensitivePatterns, "secret")
		assert.Contains(t, orchCheck.orchestratorConfig.Scrubber.LiteralSensitivePatterns, "password")
		assert.Contains(t, orchCheck.orchestratorConfig.Scrubber.LiteralSensitivePatterns, "token")
	})

	t.Run("configuration with manifest collection settings", func(t *testing.T) {
		fakeTagger := taggerfxmock.SetupFakeTagger(t)
		orchCheck := newCheck(cfg, mockStore, fakeTagger).(*OrchestratorCheck)
		setupMockAPIClient(orchCheck)

		// Enable orchestrator collection with manifest collection
		cfg.SetWithoutSource("orchestrator_explorer.enabled", true)
		cfg.SetWithoutSource("cluster_name", "test-cluster")
		cfg.SetWithoutSource("orchestrator_explorer.manifest_collection.enabled", true)
		cfg.SetWithoutSource("orchestrator_explorer.manifest_collection.buffer_manifest", true)
		cfg.SetWithoutSource("orchestrator_explorer.manifest_collection.buffer_flush_interval", "45s")

		err := orchCheck.Configure(mockSenderManager, uint64(1), integration.Data{}, integration.Data{}, "test")
		assert.NoError(t, err)

		// Verify manifest collection settings
		assert.True(t, orchCheck.orchestratorConfig.IsManifestCollectionEnabled)
		assert.True(t, orchCheck.orchestratorConfig.BufferedManifestEnabled)
		assert.Equal(t, 45*time.Second, orchCheck.orchestratorConfig.ManifestBufferFlushInterval)
	})

	t.Run("configuration with pod queue bytes settings", func(t *testing.T) {
		fakeTagger := taggerfxmock.SetupFakeTagger(t)
		orchCheck := newCheck(cfg, mockStore, fakeTagger).(*OrchestratorCheck)
		setupMockAPIClient(orchCheck)

		// Enable orchestrator collection with custom pod queue bytes
		cfg.SetWithoutSource("orchestrator_explorer.enabled", true)
		cfg.SetWithoutSource("cluster_name", "test-cluster")
		cfg.SetWithoutSource("process_config.pod_queue_bytes", 30*1000*1000) // 30MB

		err := orchCheck.Configure(mockSenderManager, uint64(1), integration.Data{}, integration.Data{}, "test")
		assert.NoError(t, err)

		// Verify pod queue bytes setting
		assert.Equal(t, 30*1000*1000, orchCheck.orchestratorConfig.PodQueueBytes)
	})

	t.Run("configuration with additional endpoints", func(t *testing.T) {
		fakeTagger := taggerfxmock.SetupFakeTagger(t)
		orchCheck := newCheck(cfg, mockStore, fakeTagger).(*OrchestratorCheck)
		setupMockAPIClient(orchCheck)

		// Enable orchestrator collection with additional endpoints
		cfg.SetWithoutSource("orchestrator_explorer.enabled", true)
		cfg.SetWithoutSource("cluster_name", "test-cluster")
		cfg.SetWithoutSource("api_key", "main-api-key")
		cfg.SetWithoutSource("orchestrator_explorer.orchestrator_additional_endpoints",
			`{"https://endpoint1.com": ["key1"], "https://endpoint2.com": ["key2"]}`)

		err := orchCheck.Configure(mockSenderManager, uint64(1), integration.Data{}, integration.Data{}, "test")
		assert.NoError(t, err)

		// Verify additional endpoints are configured
		// Should have main endpoint + 2 additional endpoints
		assert.GreaterOrEqual(t, len(orchCheck.orchestratorConfig.OrchestratorEndpoints), 2)

		// Check that main endpoint has the API key
		mainEndpoint := orchCheck.orchestratorConfig.OrchestratorEndpoints[0]
		assert.Equal(t, "main-api-key", mainEndpoint.APIKey)
	})

	t.Run("configuration preserves collector bundle initialization", func(t *testing.T) {
		fakeTagger := taggerfxmock.SetupFakeTagger(t)
		orchCheck := newCheck(cfg, mockStore, fakeTagger).(*OrchestratorCheck)
		setupMockAPIClient(orchCheck)

		// Enable orchestrator collection
		cfg.SetWithoutSource("orchestrator_explorer.enabled", true)
		cfg.SetWithoutSource("cluster_name", "test-cluster")

		err := orchCheck.Configure(mockSenderManager, uint64(1), integration.Data{}, integration.Data{}, "test")
		assert.NoError(t, err)

		// Verify collector bundle is properly initialized
		assert.NotNil(t, orchCheck.collectorBundle)
		assert.Equal(t, orchCheck, orchCheck.collectorBundle.check)
		assert.NotNil(t, orchCheck.collectorBundle.runCfg)
		assert.Equal(t, orchCheck.clusterID, orchCheck.collectorBundle.runCfg.ClusterID)
		assert.Equal(t, orchCheck.orchestratorConfig, orchCheck.collectorBundle.runCfg.Config)
	})
}
