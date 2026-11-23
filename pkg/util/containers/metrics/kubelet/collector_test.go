// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package kubelet

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/core"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	kutil "github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet/mock"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
)

func TestKubeletCollectorLinux(t *testing.T) {
	metadataStore := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		fx.Supply(context.Background()),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	kubeletMock := mock.NewKubeletMock()

	// POD UID is c84eb7fb-09f2-11ea-abb1-42010a84017a
	// Has containers kubedns, prometheus-to-sd, sidecar, dnsmasq
	setStatsSummaryFromFile(t, "./testdata/statsSummaryLinux.json", kubeletMock)
	metadataStore.Set(&workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   "c84eb7fb-09f2-11ea-abb1-42010a84017a",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:      "kube-dns-5877696fb4-m6cvp",
			Namespace: "kube-system",
		},
		Containers: []workloadmeta.OrchestratorContainer{
			{
				ID:   "cID1",
				Name: "kubedns",
			},
			{
				ID:   "cID2",
				Name: "prometheus-to-sd",
			},
			{
				ID:   "cID3",
				Name: "sidecar",
			},
		},
	})

	kubeletCollector := &kubeletCollector{
		kubeletClient:          kubeletMock,
		podContainerIDResolver: podContainerIDResolver{metadataStore: metadataStore},
	}

	// On first `GetCoreContainerStats`, the full data is read and cache is filled
	expectedTime, _ := time.Parse(time.RFC3339, "2019-11-20T13:13:13Z")
	expectedTime = expectedTime.Local()
	cID1Stats, err := kubeletCollector.GetContainerStats("", "cID1", time.Minute)
	// Removing content from kubeletMock to make sure anything we hit is from cache
	clearFakeStatsSummary(kubeletMock)

	assert.NoError(t, err)
	assert.Equal(t, &provider.ContainerStats{
		Timestamp: expectedTime,
		CPU: &provider.ContainerCPUStats{
			Total: pointer.Ptr(194414788585.0),
		},
		Memory: &provider.ContainerMemStats{
			UsageTotal: pointer.Ptr(12713984.0),
			RSS:        pointer.Ptr(12238848.0),
			WorkingSet: pointer.Ptr(12713984.0),
			Pgfault:    pointer.Ptr(13101.0),
			Pgmajfault: pointer.Ptr(12.0),
		},
	}, cID1Stats)

	expectedTime, _ = time.Parse(time.RFC3339, "2019-11-20T13:13:09Z")
	expectedTime = expectedTime.Local()
	cID2Stats, err := kubeletCollector.GetContainerStats("", "cID2", time.Minute)
	assert.NoError(t, err)
	assert.Equal(t, &provider.ContainerStats{
		Timestamp: expectedTime,
		CPU: &provider.ContainerCPUStats{
			Total: pointer.Ptr(12460233103.0),
		},
		Memory: &provider.ContainerMemStats{
			UsageTotal: pointer.Ptr(6705152.0),
			RSS:        pointer.Ptr(6119424.0),
			WorkingSet: pointer.Ptr(6705152.0),
			Pgfault:    pointer.Ptr(9603.0),
			Pgmajfault: pointer.Ptr(42.0),
		},
	}, cID2Stats)

	expectedTime, _ = time.Parse(time.RFC3339, "2019-11-20T13:13:16Z")
	expectedTime = expectedTime.Local()
	cID3Stats, err := kubeletCollector.GetContainerStats("", "cID3", time.Minute)
	assert.NoError(t, err)
	assert.Equal(t, &provider.ContainerStats{
		Timestamp: expectedTime,
		CPU: &provider.ContainerCPUStats{
			Total: pointer.Ptr(139979939975.0),
		},
		Memory: &provider.ContainerMemStats{
			UsageTotal: pointer.Ptr(11325440.0),
			RSS:        pointer.Ptr(10797056.0),
			WorkingSet: pointer.Ptr(11325440.0),
			Pgfault:    pointer.Ptr(7722.0),
			Pgmajfault: pointer.Ptr(7.0),
		},
	}, cID3Stats)

	// Test network stats
	expectedPodNetworkStats := &provider.ContainerNetworkStats{
		Timestamp: expectedTime,
		BytesRcvd: pointer.Ptr(254942755.0),
		BytesSent: pointer.Ptr(137422821.0),
		Interfaces: map[string]provider.InterfaceNetStats{
			"eth0": {
				BytesRcvd: pointer.Ptr(254942755.0),
				BytesSent: pointer.Ptr(137422821.0),
			},
		},
		NetworkIsolationGroupID: pointer.Ptr(uint64(17659160645723176180)),
	}

	cID3NetworkStats, err := kubeletCollector.GetContainerNetworkStats("", "cID3", time.Minute)
	assert.NoError(t, err)
	assert.Equal(t, expectedPodNetworkStats, cID3NetworkStats)

	cID2NetworkStats, err := kubeletCollector.GetContainerNetworkStats("", "cID2", time.Minute)
	assert.NoError(t, err)
	assert.Equal(t, expectedPodNetworkStats, cID2NetworkStats)

	// Test getting stats for an unknown container, should answer without data but without error (no API call triggered)
	cID4Stats, err := kubeletCollector.GetContainerStats("", "cID4", time.Minute)
	assert.NoError(t, err)
	assert.Nil(t, cID4Stats)

	// Forcing a refresh, will trigger a Kubelet call (which will answer with 404 Not found)
	cID1Stats, err = kubeletCollector.GetContainerStats("", "cID1", 0)
	assert.Equal(t, err.Error(), "Unable to fetch stats summary from Kubelet, rc: 404")
	assert.Nil(t, cID1Stats)
}

func TestKubeletCollectorWindows(t *testing.T) {
	metadataStore := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		fx.Supply(context.Background()),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
	kubeletMock := mock.NewKubeletMock()

	// POD UID is 8ddf0e3f-ac6c-4d44-87d7-0bc41f6729ec
	// Has containers trace-agent, agent, process-agent
	setStatsSummaryFromFile(t, "./testdata/statsSummaryWindows.json", kubeletMock)
	metadataStore.Set(&workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   "8ddf0e3f-ac6c-4d44-87d7-0bc41f6729ec",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:      "dd-datadog-lbvkl",
			Namespace: "default",
		},
		Containers: []workloadmeta.OrchestratorContainer{
			{
				ID:   "cID1",
				Name: "process-agent",
			},
		},
	})

	kubeletCollector := &kubeletCollector{
		kubeletClient:          kubeletMock,
		podContainerIDResolver: podContainerIDResolver{metadataStore: metadataStore},
	}

	// On first `GetCoreContainerStats`, the full data is read and cache is filled
	expectedTime, _ := time.Parse(time.RFC3339, "2020-04-24T15:54:14Z")
	expectedTime = expectedTime.Local()
	cID1Stats, err := kubeletCollector.GetContainerStats("", "cID1", time.Minute)
	assert.NoError(t, err)
	assert.Equal(t, &provider.ContainerStats{
		Timestamp: expectedTime,
		CPU: &provider.ContainerCPUStats{
			Total: pointer.Ptr(9359375000.0),
		},
		Memory: &provider.ContainerMemStats{
			UsageTotal:        pointer.Ptr(65474560.0),
			PrivateWorkingSet: pointer.Ptr(65474560.0),
		},
	}, cID1Stats)
}

func TestContainerIDForPodUIDAndContName(t *testing.T) {
	for _, tt := range []struct {
		name        string
		pod         *workloadmeta.KubernetesPod
		podUID      string
		contName    string
		initCont    bool
		expectedCid string
	}{
		{
			name: "pod with container",
			pod: &workloadmeta.KubernetesPod{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesPod,
					ID:   "c84eb7fb-09f2-11ea-abb1-42010a84017a",
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name:      "kube-dns-5877696fb4-m6cvp",
					Namespace: "kube-system",
				},
				Containers: []workloadmeta.OrchestratorContainer{
					{
						ID:   "cID1",
						Name: "kubedns",
					},
				},
			},
			podUID:      "c84eb7fb-09f2-11ea-abb1-42010a84017a",
			contName:    "kubedns",
			initCont:    false,
			expectedCid: "cID1",
		},
		{
			name: "pod with init container",
			pod: &workloadmeta.KubernetesPod{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesPod,
					ID:   "c84eb7fb-09f2-11ea-abb1-42010a84017a",
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name:      "kube-dns-5877696fb4-m6cvp",
					Namespace: "kube-system",
				},
				InitContainers: []workloadmeta.OrchestratorContainer{
					{
						ID:   "cID1",
						Name: "kubedns",
					},
				},
			},
			podUID:      "c84eb7fb-09f2-11ea-abb1-42010a84017a",
			contName:    "kubedns",
			initCont:    true,
			expectedCid: "cID1",
		},
		{
			name: "pod with ephemeral container",
			pod: &workloadmeta.KubernetesPod{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesPod,
					ID:   "c84eb7fb-09f2-11ea-abb1-42010a84017a",
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name:      "kube-dns-5877696fb4-m6cvp",
					Namespace: "kube-system",
				},
				EphemeralContainers: []workloadmeta.OrchestratorContainer{
					{
						ID:   "ephemeralID",
						Name: "ephemeralContainer",
					},
				},
			},
			podUID:      "c84eb7fb-09f2-11ea-abb1-42010a84017a",
			contName:    "ephemeralContainer",
			initCont:    false,
			expectedCid: "ephemeralID",
		},
		{
			name: "native sidecar found without init flag",
			pod: &workloadmeta.KubernetesPod{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesPod,
					ID:   "c84eb7fb-09f2-11ea-abb1-42010a84017a",
				},
				Containers: []workloadmeta.OrchestratorContainer{
					{
						ID:   "appID",
						Name: "app",
					},
				},
				InitContainers: []workloadmeta.OrchestratorContainer{
					{
						ID:   "sidecarID",
						Name: "sidecar",
					},
				},
			},
			podUID:      "c84eb7fb-09f2-11ea-abb1-42010a84017a",
			contName:    "sidecar",
			initCont:    false,
			expectedCid: "sidecarID",
		},
		{
			name: "init flag ignored for regular container",
			pod: &workloadmeta.KubernetesPod{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesPod,
					ID:   "c84eb7fb-09f2-11ea-abb1-42010a84017a",
				},
				Containers: []workloadmeta.OrchestratorContainer{
					{
						ID:   "appID",
						Name: "app",
					},
				},
				InitContainers: []workloadmeta.OrchestratorContainer{
					{
						ID:   "sidecarID",
						Name: "sidecar",
					},
				},
			},
			podUID:      "c84eb7fb-09f2-11ea-abb1-42010a84017a",
			contName:    "app",
			initCont:    true,
			expectedCid: "appID",
		},
		{
			name: "init flag ignored for ephemeral container",
			pod: &workloadmeta.KubernetesPod{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesPod,
					ID:   "c84eb7fb-09f2-11ea-abb1-42010a84017a",
				},
				EphemeralContainers: []workloadmeta.OrchestratorContainer{
					{
						ID:   "ephemeralID",
						Name: "ephemeralContainer",
					},
				},
			},
			podUID:      "c84eb7fb-09f2-11ea-abb1-42010a84017a",
			contName:    "ephemeralContainer",
			initCont:    true,
			expectedCid: "ephemeralID",
		},
		{
			name: "container name not in pod",
			pod: &workloadmeta.KubernetesPod{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesPod,
					ID:   "c84eb7fb-09f2-11ea-abb1-42010a84017a",
				},
				Containers: []workloadmeta.OrchestratorContainer{
					{
						ID:   "appID",
						Name: "app",
					},
				},
				InitContainers: []workloadmeta.OrchestratorContainer{
					{
						ID:   "sidecarID",
						Name: "sidecar",
					},
				},
			},
			podUID:      "c84eb7fb-09f2-11ea-abb1-42010a84017a",
			contName:    "missing",
			initCont:    false,
			expectedCid: "",
		},
		{
			name:        "not found",
			podUID:      "poduid",
			contName:    "contname",
			initCont:    false,
			expectedCid: "",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			metadataStore := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
				core.MockBundle(),
				fx.Supply(context.Background()),
				workloadmetafxmock.MockModule(workloadmeta.NewParams()),
			))

			kubeletMock := mock.NewKubeletMock()
			if tt.pod != nil {
				metadataStore.Set(tt.pod)
			}

			kubeletCollector := &kubeletCollector{
				kubeletClient:          kubeletMock,
				podContainerIDResolver: podContainerIDResolver{metadataStore: metadataStore},
			}

			cid, err := kubeletCollector.ContainerIDForPodUIDAndContName(tt.podUID, tt.contName, tt.initCont, time.Minute)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedCid, cid)
		})
	}
}

func TestNewKubeletCollector(t *testing.T) {
	errKubeletUnreachable := errors.New("kubelet unreachable")

	for _, tt := range []struct {
		name                string
		notOnKubernetes     bool
		flavor              string
		clcRunner           bool
		otelStandalone      bool
		useKubeletCollector bool
		kubeUtilErr         error
		// wantClient tells whether the collector creates a kubelet client and
		// serves container stats from it.
		wantClient bool
		wantErr    error
	}{
		{
			name:            "not on Kubernetes",
			notOnKubernetes: true,
			flavor:          flavor.DefaultAgent,
			wantErr:         provider.ErrPermaFail,
		},
		{
			name:      "cluster checks runner",
			flavor:    flavor.DefaultAgent,
			clcRunner: true,
			wantErr:   provider.ErrPermaFail,
		},
		{
			name:           "otel-agent standalone with the nodefilter collector",
			flavor:         flavor.OTelAgent,
			otelStandalone: true,
		},
		{
			name:                "otel-agent standalone with the kubelet collector",
			flavor:              flavor.OTelAgent,
			otelStandalone:      true,
			useKubeletCollector: true,
			wantClient:          true,
		},
		{
			name:       "otel-agent connected",
			flavor:     flavor.OTelAgent,
			wantClient: true,
		},
		{
			name:           "core agent ignores otel_standalone",
			flavor:         flavor.DefaultAgent,
			otelStandalone: true,
			wantClient:     true,
		},
		{
			name:        "core agent with an unreachable kubelet is retried",
			flavor:      flavor.DefaultAgent,
			kubeUtilErr: errKubeletUnreachable,
			wantClient:  true,
			wantErr:     errKubeletUnreachable,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.notOnKubernetes {
				env.SetFeatures(t)
			} else {
				env.SetFeatures(t, env.Kubernetes)
			}
			flavor.SetTestFlavor(t, tt.flavor)
			cfg := configmock.New(t)
			if tt.clcRunner {
				cfg.SetInTest("clc_runner_enabled", true)
				cfg.SetInTest("config_providers", []map[string]interface{}{{"name": "clusterchecks"}})
			}
			cfg.SetInTest("otel_standalone", tt.otelStandalone)
			cfg.SetInTest("otelcollector.standalone.use_kubelet_collector", tt.useKubeletCollector)

			kubeUtilCalled := false
			originalGetKubeUtil := getKubeUtil
			t.Cleanup(func() { getKubeUtil = originalGetKubeUtil })
			getKubeUtil = func() (kutil.KubeUtilInterface, error) {
				kubeUtilCalled = true
				if tt.kubeUtilErr != nil {
					return nil, tt.kubeUtilErr
				}
				return mock.NewKubeletMock(), nil
			}

			metadataStore := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
				core.MockBundle(),
				fx.Supply(context.Background()),
				workloadmetafxmock.MockModule(workloadmeta.NewParams()),
			))
			metadataStore.Set(&workloadmeta.KubernetesPod{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesPod,
					ID:   "pod-uid",
				},
				Containers: []workloadmeta.OrchestratorContainer{
					{ID: "container-id", Name: "app"},
				},
			})

			collectorMetadata, err := newKubeletCollector(nil, metadataStore)
			assert.Equal(t, tt.wantClient, kubeUtilCalled, "whether a kubelet client is created")
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				assert.Empty(t, collectorMetadata.Collectors)
				return
			}
			require.NoError(t, err)

			assert.Equal(t, collectorID, collectorMetadata.ID)
			require.NotEmpty(t, collectorMetadata.Collectors)
			for runtime, collectors := range collectorMetadata.Collectors {
				if tt.wantClient {
					assert.NotNil(t, collectors.Stats.Collector, runtime)
					assert.NotNil(t, collectors.Network.Collector, runtime)
				} else {
					assert.Nil(t, collectors.Stats.Collector, runtime)
					assert.Nil(t, collectors.Network.Collector, runtime)
				}

				require.NotNil(t, collectors.ContainerIDForPodUIDAndContName.Collector, runtime)
				cid, err := collectors.ContainerIDForPodUIDAndContName.Collector.ContainerIDForPodUIDAndContName("pod-uid", "app", false, time.Minute)
				assert.NoError(t, err, runtime)
				assert.Equal(t, "container-id", cid, runtime)
			}
		})
	}
}

func setStatsSummaryFromFile(t *testing.T, filePath string, kubeletMock *mock.KubeletMock) {
	t.Helper()

	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Errorf("unable to read test file at: %s, err: %v", filePath, err)
	}

	setFakeStatsSummary(kubeletMock, content, 200, nil)
}

func setFakeStatsSummary(kubeletMock *mock.KubeletMock, content []byte, rc int, err error) {
	kubeletMock.MockReplies["/stats/summary"] = &mock.HTTPReplyMock{
		Data:         content,
		ResponseCode: rc,
		Error:        err,
	}
}

func clearFakeStatsSummary(kubeletMock *mock.KubeletMock) {
	delete(kubeletMock.MockReplies, "/stats/summary")
}
