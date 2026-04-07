// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && kubelet

package kubemetadata

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	k8sschema "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	apiv1 "github.com/DataDog/datadog-agent/pkg/clusteragent/api/v1"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/process"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/version"
)

type fakeKubeUtil struct {
	kubelet.KubeUtil
	nodeName string
}

func (f *fakeKubeUtil) GetNodename(_ context.Context) (string, error) { return f.nodeName, nil }

type FakeDCAClient struct {
	LocalVersion                 version.Version
	LocalClusterAgentAPIEndpoint string

	VersionErr error

	NodeLabels    map[string]string
	NodeLabelsErr error

	NodeAnnotations    map[string]string
	NodeAnnotationsErr error

	NodeUID    string
	NodeUIDErr error

	NamespaceLabels    map[string]string
	NamespaceLabelsErr error

	NamespaceMetadata    clusteragent.Metadata
	NamespaceMetadataErr error

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

func (f *FakeDCAClient) Version(_ bool) version.Version {
	return f.LocalVersion
}

func (f *FakeDCAClient) ClusterAgentAPIEndpoint() string {
	return f.LocalClusterAgentAPIEndpoint
}

func (f *FakeDCAClient) GetNodeLabels(_ string) (map[string]string, error) {
	return f.NodeLabels, f.NodeLabelsErr
}

func (f *FakeDCAClient) GetNodeAnnotations(_ string, _ ...string) (map[string]string, error) {
	return f.NodeAnnotations, f.NodeLabelsErr
}

func (f *FakeDCAClient) GetNodeInfo(_ string, _ ...string) (*clusteragent.NodeSystemInfo, error) {
	panic("implement me")
}

func (f *FakeDCAClient) GetNodeUID(_ string) (string, error) {
	return f.NodeUID, f.NodeUIDErr
}

func (f *FakeDCAClient) GetNamespaceLabels(_ string) (map[string]string, error) {
	return f.NamespaceLabels, f.NamespaceLabelsErr
}

func (f *FakeDCAClient) GetNamespaceMetadata(_ string) (*clusteragent.Metadata, error) {
	return &f.NamespaceMetadata, f.NamespaceMetadataErr
}

func (f *FakeDCAClient) GetPodsMetadataForNode(_ string) (apiv1.NamespacesPodsStringsSet, error) {
	return f.PodMetadataForNode, f.PodMetadataForNodeErr
}

func (f *FakeDCAClient) GetKubernetesMetadataNames(_, _, _ string) ([]string, error) {
	return f.KubernetesMetadataNames, f.KubernetesMetadataNamesErr
}

func (f *FakeDCAClient) PostClusterCheckStatus(_ context.Context, _ string, _ types.NodeStatus) (types.StatusResponse, error) {
	return f.ClusterCheckStatus, f.ClusterCheckStatusErr
}

func (f *FakeDCAClient) GetClusterCheckConfigs(_ context.Context, _ string) (types.ConfigResponse, error) {
	return f.ClusterCheckConfigs, f.ClusterCheckConfigsErr
}

func (f *FakeDCAClient) GetEndpointsCheckConfigs(_ context.Context, _ string) (types.ConfigResponse, error) {
	return f.EndpointsCheckConfigs, f.EndpointsCheckConfigsErr
}

func (f *FakeDCAClient) GetKubernetesClusterID() (string, error) {
	return f.ClusterID, f.ClusterIDErr
}

func (f *FakeDCAClient) GetCFAppsMetadataForNode(_ string) (map[string][]string, error) {
	panic("implement me")
}

func (f *FakeDCAClient) PostLanguageMetadata(_ context.Context, _ *pbgo.ParentLanguageAnnotationRequest) error {
	panic("implement me")
}

func (f *FakeDCAClient) SupportsNamespaceMetadataCollection() bool {
	return f.LocalVersion.Major >= 7 && f.LocalVersion.Minor >= 55
}

func TestCollector_selectPullBasedProvider(t *testing.T) {
	tests := []struct {
		name      string
		collector collector
		wantType  interface{}
	}{
		{
			name: "local apiserver provider when DCA is disabled",
			collector: collector{
				dcaClient: nil,
			},
			wantType: &localAPIServerProvider{},
		},
		{
			name: "per-pod provider for DCA < 1.3",
			collector: collector{
				dcaEnabled: true,
				dcaClient: &FakeDCAClient{
					LocalVersion: version.Version{Major: 1, Minor: 2},
				},
			},
			wantType: &dcaPerPodProvider{},
		},
		{
			name: "per-node provider for DCA >= 1.3 and < 7.55",
			collector: collector{
				dcaEnabled: true,
				kubeUtil:   &fakeKubeUtil{nodeName: "node-a"},
				dcaClient: &FakeDCAClient{
					LocalVersion: version.Version{Major: 7, Minor: 54},
				},
			},
			wantType: &dcaPerNodeProvider{},
		},
		{
			name: "full provider for DCA >= 7.55",
			collector: collector{
				dcaEnabled: true,
				kubeUtil:   &fakeKubeUtil{nodeName: "node-a"},
				dcaClient: &FakeDCAClient{
					LocalVersion: version.Version{Major: 7, Minor: 55},
				},
			},
			wantType: &dcaFullProvider{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := tt.collector.selectPullBasedProvider(context.TODO())
			assert.NoError(t, err)
			assert.IsType(t, tt.wantType, provider)
		})
	}
}

func TestCollector_detectExpiredNamespace(t *testing.T) {
	tests := []struct {
		name              string
		seenID            workloadmeta.EntityID
		namespaceLastSeen map[string]time.Time
		isNsEntity        bool
		keepAlive         bool
		wantCleanup       bool // whether the namespace should be removed from tracking
	}{
		{
			name: "non-metadata entity returns false",
			seenID: workloadmeta.EntityID{
				Kind: workloadmeta.KindKubernetesPod,
				ID:   "some-pod-id",
			},
			namespaceLastSeen: map[string]time.Time{},
			isNsEntity:        false,
			keepAlive:         false,
			wantCleanup:       false,
		},
		{
			name: "metadata entity but not a namespace returns false",
			seenID: workloadmeta.EntityID{
				Kind: workloadmeta.KindKubernetesMetadata,
				ID:   "/deployments/default/my-deployment",
			},
			namespaceLastSeen: map[string]time.Time{},
			isNsEntity:        false,
			keepAlive:         false,
			wantCleanup:       false,
		},
		{
			name: "namespace within TTL returns true",
			seenID: workloadmeta.EntityID{
				Kind: workloadmeta.KindKubernetesMetadata,
				ID:   "/namespaces//my-namespace",
			},
			namespaceLastSeen: map[string]time.Time{
				"my-namespace": time.Now().Add(-1 * namespaceMetadataTTL / 100), // 0.01 TTLs ago (unexpired)
			},
			isNsEntity:  true,
			keepAlive:   true,
			wantCleanup: false,
		},
		{
			name: "namespace expired returns false and cleans up",
			seenID: workloadmeta.EntityID{
				Kind: workloadmeta.KindKubernetesMetadata,
				ID:   "/namespaces//expired-namespace",
			},
			namespaceLastSeen: map[string]time.Time{
				"expired-namespace": time.Now().Add(-2 * namespaceMetadataTTL), // 2 TTLs ago (expired)
			},
			isNsEntity:  true,
			keepAlive:   false,
			wantCleanup: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &collector{
				namespaceLastSeen: tt.namespaceLastSeen,
			}

			namespaceName, isNsEntity := c.getNamespaceName(tt.seenID)
			assert.Equal(t, tt.isNsEntity, isNsEntity)

			if tt.isNsEntity {
				keepAlive := c.shouldKeepNamespaceAlive(namespaceName)
				assert.Equal(t, tt.keepAlive, keepAlive)
			}

			if tt.wantCleanup {
				assert.Empty(t, c.namespaceLastSeen, "expired namespace should be removed from tracking")
			}
		})
	}
}

func TestCollector_createUnsetEvent(t *testing.T) {
	tests := []struct {
		name       string
		seenID     workloadmeta.EntityID
		wantEntity workloadmeta.Entity
	}{
		{
			name: "pod entity",
			seenID: workloadmeta.EntityID{
				Kind: workloadmeta.KindKubernetesPod,
				ID:   "pod-uid-123",
			},
			wantEntity: &workloadmeta.KubernetesPod{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesPod,
					ID:   "pod-uid-123",
				},
			},
		},
		{
			name: "kubernetes metadata entity",
			seenID: workloadmeta.EntityID{
				Kind: workloadmeta.KindKubernetesMetadata,
				ID:   "/namespaces//my-namespace",
			},
			wantEntity: &workloadmeta.KubernetesMetadata{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesMetadata,
					ID:   "/namespaces//my-namespace",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := createUnsetEvent(tt.seenID)

			assert.Equal(t, workloadmeta.EventTypeUnset, event.Type)
			assert.Equal(t, workloadmeta.SourceClusterOrchestrator, event.Source)
			assert.Equal(t, tt.wantEntity, event.Entity)
		})
	}
}

func TestKubeMetadataCollector_parsePods(t *testing.T) {
	pods := []*kubelet.Pod{{
		Metadata: kubelet.PodMetadata{
			Name:      "foo",
			Namespace: "default",
			UID:       "foouid",
		},
		Spec: kubelet.Spec{
			NodeName: "nodename",
		},
		Status: kubelet.Status{
			Phase: "Running",
			Conditions: []kubelet.Conditions{
				{
					Type:   "Ready",
					Status: "True",
				},
			},
		},
	}}
	podsCache := kubelet.PodList{
		Items: pods,
	}
	podMetadata := apiv1.NamespacesPodsStringsSet{
		"default": apiv1.MapStringSet{
			"foo": sets.New("svc1", "svc2"),
		},
	}

	type fields struct {
		apiClient                   *apiserver.APIClient
		dcaClient                   clusteragent.DCAClientInterface
		lastUpdate                  time.Time
		updateFreq                  time.Duration
		dcaEnabled                  bool
		collectNamespaceLabels      bool
		collectNamespaceAnnotations bool
	}
	type args struct {
		pods []*kubelet.Pod
	}
	tests := []struct {
		name                       string
		fields                     fields
		args                       args
		namespaceLabelsAsTags      map[string]string
		namespaceAnnotationsAsTags map[string]string
		want                       []workloadmeta.CollectorEvent
		wantErr                    bool
	}{
		{
			name: "clusterAgentEnabled enabled, cluster-agent 1.3.x >=",
			args: args{
				pods: pods,
			},
			fields: fields{
				dcaEnabled:             true,
				collectNamespaceLabels: true,
				dcaClient: &FakeDCAClient{
					LocalVersion:       version.Version{Major: 1, Minor: 3},
					PodMetadataForNode: podMetadata,
					NamespaceLabels: map[string]string{
						"label": "value",
					},
				},
			},
			namespaceLabelsAsTags: map[string]string{
				"label": "tag",
			},
			want: []workloadmeta.CollectorEvent{
				{
					Type:   workloadmeta.EventTypeSet,
					Source: workloadmeta.SourceClusterOrchestrator,
					Entity: &workloadmeta.KubernetesPod{
						EntityID: workloadmeta.EntityID{
							Kind: workloadmeta.KindKubernetesPod,
							ID:   "foouid",
						},
						EntityMeta: workloadmeta.EntityMeta{
							Name:      "foo",
							Namespace: "default",
						},
						KubeServices: []string{"svc1", "svc2"},
						NamespaceLabels: map[string]string{
							"label": "value",
						},
					},
				},
				{
					Type:   workloadmeta.EventTypeSet,
					Source: workloadmeta.SourceClusterOrchestrator,
					Entity: &workloadmeta.KubernetesMetadata{
						EntityID: workloadmeta.EntityID{
							Kind: workloadmeta.KindKubernetesMetadata,
							ID:   "/namespaces//default",
						},
						EntityMeta: workloadmeta.EntityMeta{
							Name:      "default",
							Namespace: "",
							Labels: map[string]string{
								"label": "value",
							},
							Annotations: map[string]string{},
						},
						GVR: &k8sschema.GroupVersionResource{
							Group:    "",
							Version:  "v1",
							Resource: "namespaces",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "cluster agent not enabled, ns labels enabled, ns annotations enabled",
			args: args{
				pods: pods,
			},
			fields: fields{
				dcaEnabled:                  false,
				collectNamespaceLabels:      true,
				collectNamespaceAnnotations: true,
				dcaClient:                   nil,
			},
			namespaceLabelsAsTags: map[string]string{
				"label": "tag",
			},
			namespaceAnnotationsAsTags: map[string]string{
				"annotation": "tag",
			},
			want: []workloadmeta.CollectorEvent{
				{
					Type:   workloadmeta.EventTypeSet,
					Source: workloadmeta.SourceClusterOrchestrator,
					Entity: &workloadmeta.KubernetesPod{
						EntityID: workloadmeta.EntityID{
							Kind: workloadmeta.KindKubernetesPod,
							ID:   "foouid",
						},
						EntityMeta: workloadmeta.EntityMeta{
							Name:      "foo",
							Namespace: "default",
						},
						KubeServices: []string{},
					},
				},
				{
					Type:   workloadmeta.EventTypeSet,
					Source: workloadmeta.SourceClusterOrchestrator,
					Entity: &workloadmeta.KubernetesMetadata{
						EntityID: workloadmeta.EntityID{
							Kind: workloadmeta.KindKubernetesMetadata,
							ID:   "/namespaces//default",
						},
						EntityMeta: workloadmeta.EntityMeta{
							Name:        "default",
							Namespace:   "",
							Labels:      map[string]string{},
							Annotations: map[string]string{},
						},
						GVR: &k8sschema.GroupVersionResource{
							Group:    "",
							Version:  "v1",
							Resource: "namespaces",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "clusterAgentEnabled enabled, cluster-agent version < 7.55, ns labels enabled, ns annotations enabled",
			args: args{
				pods: pods,
			},
			fields: fields{
				dcaEnabled:                  true,
				collectNamespaceLabels:      true,
				collectNamespaceAnnotations: true,
				dcaClient: &FakeDCAClient{
					LocalVersion:       version.Version{Major: 1, Minor: 3},
					PodMetadataForNode: podMetadata,
					NamespaceLabels: map[string]string{
						"label": "value",
					},
				},
			},
			namespaceLabelsAsTags: map[string]string{
				"label": "tag",
			},
			namespaceAnnotationsAsTags: map[string]string{
				"annotation": "tag",
			},
			want: []workloadmeta.CollectorEvent{
				{
					Type:   workloadmeta.EventTypeSet,
					Source: workloadmeta.SourceClusterOrchestrator,
					Entity: &workloadmeta.KubernetesPod{
						EntityID: workloadmeta.EntityID{
							Kind: workloadmeta.KindKubernetesPod,
							ID:   "foouid",
						},
						EntityMeta: workloadmeta.EntityMeta{
							Name:      "foo",
							Namespace: "default",
						},
						KubeServices: []string{"svc1", "svc2"},
						NamespaceLabels: map[string]string{
							"label": "value",
						},
					},
				},
				{
					Type:   workloadmeta.EventTypeSet,
					Source: workloadmeta.SourceClusterOrchestrator,
					Entity: &workloadmeta.KubernetesMetadata{
						EntityID: workloadmeta.EntityID{
							Kind: workloadmeta.KindKubernetesMetadata,
							ID:   "/namespaces//default",
						},
						EntityMeta: workloadmeta.EntityMeta{
							Name:      "default",
							Namespace: "",
							Labels: map[string]string{
								"label": "value",
							},
							Annotations: map[string]string{},
						},
						GVR: &k8sschema.GroupVersionResource{
							Group:    "",
							Version:  "v1",
							Resource: "namespaces",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "clusterAgentEnabled enabled, cluster-agent version >= 7.55, ns labels collection enabled, ns annotations collection enabled",
			args: args{
				pods: pods,
			},
			fields: fields{
				dcaEnabled:                  true,
				collectNamespaceLabels:      true,
				collectNamespaceAnnotations: true,
				dcaClient: &FakeDCAClient{
					LocalVersion:       version.Version{Major: 7, Minor: 55},
					PodMetadataForNode: podMetadata,
					NamespaceLabels: map[string]string{
						"label": "value",
					},
					NamespaceMetadata: clusteragent.Metadata{
						Labels: map[string]string{
							"label": "value",
						},
						Annotations: map[string]string{
							"annotation": "value",
						},
					},
				},
			},
			namespaceLabelsAsTags: map[string]string{
				"label": "label-tag",
			},
			namespaceAnnotationsAsTags: map[string]string{
				"annotation": "annotation-tag",
			},
			want: []workloadmeta.CollectorEvent{
				{
					Type:   workloadmeta.EventTypeSet,
					Source: workloadmeta.SourceClusterOrchestrator,
					Entity: &workloadmeta.KubernetesPod{
						EntityID: workloadmeta.EntityID{
							Kind: workloadmeta.KindKubernetesPod,
							ID:   "foouid",
						},
						EntityMeta: workloadmeta.EntityMeta{
							Name:      "foo",
							Namespace: "default",
						},
						KubeServices: []string{"svc1", "svc2"},
						NamespaceLabels: map[string]string{
							"label": "value",
						},
						NamespaceAnnotations: map[string]string{
							"annotation": "value",
						},
					},
				},
				{
					Type:   workloadmeta.EventTypeSet,
					Source: workloadmeta.SourceClusterOrchestrator,
					Entity: &workloadmeta.KubernetesMetadata{
						EntityID: workloadmeta.EntityID{
							Kind: workloadmeta.KindKubernetesMetadata,
							ID:   "/namespaces//default",
						},
						EntityMeta: workloadmeta.EntityMeta{
							Name:      "default",
							Namespace: "",
							Labels: map[string]string{
								"label": "value",
							},
							Annotations: map[string]string{
								"annotation": "value",
							},
						},
						GVR: &k8sschema.GroupVersionResource{
							Group:    "",
							Version:  "v1",
							Resource: "namespaces",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "clusterAgentEnabled enabled, cluster-agent version >= 7.55, ns labels collection enabled, ns annotations collection disabled",
			args: args{
				pods: pods,
			},
			fields: fields{
				dcaEnabled:                  true,
				collectNamespaceLabels:      true,
				collectNamespaceAnnotations: false,
				dcaClient: &FakeDCAClient{
					LocalVersion:       version.Version{Major: 7, Minor: 55},
					PodMetadataForNode: podMetadata,
					NamespaceLabels: map[string]string{
						"label": "value",
					},
					NamespaceMetadata: clusteragent.Metadata{
						Labels: map[string]string{
							"label": "value",
						},
						Annotations: map[string]string{
							"annotation": "value",
						},
					},
				},
			},
			namespaceLabelsAsTags: map[string]string{
				"label": "label-tag",
			},
			namespaceAnnotationsAsTags: map[string]string{
				"annotation": "annotation-tag",
			},
			want: []workloadmeta.CollectorEvent{
				{
					Type:   workloadmeta.EventTypeSet,
					Source: workloadmeta.SourceClusterOrchestrator,
					Entity: &workloadmeta.KubernetesPod{
						EntityID: workloadmeta.EntityID{
							Kind: workloadmeta.KindKubernetesPod,
							ID:   "foouid",
						},
						EntityMeta: workloadmeta.EntityMeta{
							Name:      "foo",
							Namespace: "default",
						},
						KubeServices: []string{"svc1", "svc2"},
						NamespaceLabels: map[string]string{
							"label": "value",
						},
					},
				},
				{
					Type:   workloadmeta.EventTypeSet,
					Source: workloadmeta.SourceClusterOrchestrator,
					Entity: &workloadmeta.KubernetesMetadata{
						EntityID: workloadmeta.EntityID{
							Kind: workloadmeta.KindKubernetesMetadata,
							ID:   "/namespaces//default",
						},
						EntityMeta: workloadmeta.EntityMeta{
							Name:      "default",
							Namespace: "",
							Labels: map[string]string{
								"label": "value",
							},
							Annotations: map[string]string{
								"annotation": "value",
							},
						},
						GVR: &k8sschema.GroupVersionResource{
							Group:    "",
							Version:  "v1",
							Resource: "namespaces",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "clusterAgentEnabled enabled, ns labels disabled",
			args: args{
				pods: pods,
			},
			fields: fields{
				dcaEnabled:             true,
				collectNamespaceLabels: false,
				dcaClient: &FakeDCAClient{
					LocalVersion:       version.Version{Major: 1, Minor: 3},
					PodMetadataForNode: podMetadata,
					NamespaceLabels: map[string]string{
						"label": "value",
					},
				},
			},
			namespaceLabelsAsTags: map[string]string{
				"label": "tag",
			},
			want: []workloadmeta.CollectorEvent{
				{
					Type:   workloadmeta.EventTypeSet,
					Source: workloadmeta.SourceClusterOrchestrator,
					Entity: &workloadmeta.KubernetesPod{
						EntityID: workloadmeta.EntityID{
							Kind: workloadmeta.KindKubernetesPod,
							ID:   "foouid",
						},
						EntityMeta: workloadmeta.EntityMeta{
							Name:      "foo",
							Namespace: "default",
						},
						KubeServices: []string{"svc1", "svc2"},
					},
				},
				{
					Type:   workloadmeta.EventTypeSet,
					Source: workloadmeta.SourceClusterOrchestrator,
					Entity: &workloadmeta.KubernetesMetadata{
						EntityID: workloadmeta.EntityID{
							Kind: workloadmeta.KindKubernetesMetadata,
							ID:   "/namespaces//default",
						},
						EntityMeta: workloadmeta.EntityMeta{
							Name:        "default",
							Namespace:   "",
							Labels:      map[string]string{},
							Annotations: map[string]string{},
						},
						GVR: &k8sschema.GroupVersionResource{
							Group:    "",
							Version:  "v1",
							Resource: "namespaces",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "clusterAgentEnabled enabled, but client init failed",
			args: args{
				pods: pods,
			},
			fields: fields{
				dcaEnabled: true,
				dcaClient:  &FakeDCAClient{},
			},
			want: []workloadmeta.CollectorEvent{
				{
					Type:   workloadmeta.EventTypeSet,
					Source: workloadmeta.SourceClusterOrchestrator,
					Entity: &workloadmeta.KubernetesPod{
						EntityID: workloadmeta.EntityID{
							Kind: workloadmeta.KindKubernetesPod,
							ID:   "foouid",
						},
						EntityMeta: workloadmeta.EntityMeta{
							Name:      "foo",
							Namespace: "default",
						},
						KubeServices: []string{},
					},
				},
				{
					Type:   workloadmeta.EventTypeSet,
					Source: workloadmeta.SourceClusterOrchestrator,
					Entity: &workloadmeta.KubernetesMetadata{
						EntityID: workloadmeta.EntityID{
							Kind: workloadmeta.KindKubernetesMetadata,
							ID:   "/namespaces//default",
						},
						EntityMeta: workloadmeta.EntityMeta{
							Name:        "default",
							Namespace:   "",
							Labels:      map[string]string{},
							Annotations: map[string]string{},
						},
						GVR: &k8sschema.GroupVersionResource{
							Group:    "",
							Version:  "v1",
							Resource: "namespaces",
						},
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Cache never expires because the unit tests below are not covering the case
			// of cache miss. They are only testing parsePods behaves correctly depending
			// on the cluster agent version and the agent configuration.
			mockConfig := configmock.New(t)
			mockConfig.SetWithoutSource("kubelet_cache_pods_duration", 5) // Cache is disabled by default. Enable it.
			cache.Cache.Set("KubeletPodListCacheKey", podsCache, -1)

			kubeUtilFake := kubelet.NewKubeUtil()

			c := &collector{
				kubeUtil:                    kubeUtilFake,
				apiClient:                   tt.fields.apiClient,
				dcaClient:                   tt.fields.dcaClient,
				lastUpdate:                  tt.fields.lastUpdate,
				updateFreq:                  tt.fields.updateFreq,
				dcaEnabled:                  tt.fields.dcaEnabled,
				collectNamespaceLabels:      tt.fields.collectNamespaceLabels,
				collectNamespaceAnnotations: tt.fields.collectNamespaceAnnotations,
				seen:                        make(map[workloadmeta.EntityID]struct{}),
				namespaceLastSeen:           make(map[string]time.Time),
			}

			got, err := c.parsePods(context.TODO(), tt.args.pods, make(map[workloadmeta.EntityID]struct{}))
			assert.True(t, (err != nil) == tt.wantErr)
			assert.ElementsMatch(t, tt.want, got)
		})
	}
}
