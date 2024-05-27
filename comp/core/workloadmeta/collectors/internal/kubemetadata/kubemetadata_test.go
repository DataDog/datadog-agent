// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && kubelet

package kubemetadata

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	apiv1 "github.com/DataDog/datadog-agent/pkg/clusteragent/api/v1"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/version"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/process"
)

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

func (f *FakeDCAClient) Version() version.Version {
	return f.LocalVersion
}

func (f *FakeDCAClient) ClusterAgentAPIEndpoint() string {
	return f.LocalClusterAgentAPIEndpoint
}

func (f *FakeDCAClient) GetNodeLabels(_ string) (map[string]string, error) {
	return f.NodeLabels, f.NodeLabelsErr
}

func (f *FakeDCAClient) GetNodeAnnotations(_ string) (map[string]string, error) {
	return f.NodeAnnotations, f.NodeLabelsErr
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

func TestKubeMetadataCollector_getMetadata(t *testing.T) {
	type fields struct {
		dcaClient           clusteragent.DCAClientInterface
		clusterAgentEnabled bool
	}
	type args struct {
		getPodMetaDataFromAPIServerFunc func(string, string, string) ([]string, error)
		metadataByNsPods                apiv1.NamespacesPodsStringsSet
		po                              *kubelet.Pod
	}

	tests := []struct {
		name    string
		fields  fields
		args    args
		want    []string
		wantErr bool
	}{
		{
			name: "clusterAgentEnabled not enable",
			args: args{
				getPodMetaDataFromAPIServerFunc: func(string, string, string) ([]string, error) {
					return []string{"foo=bar"}, nil
				},
				po: &kubelet.Pod{},
			},
			fields: fields{
				clusterAgentEnabled: false,
				dcaClient:           &FakeDCAClient{},
			},
			want:    []string{"foo=bar"},
			wantErr: false,
		},

		{
			name: "clusterAgentEnabled not enable, APIserver return error",
			args: args{
				getPodMetaDataFromAPIServerFunc: func(string, string, string) ([]string, error) {
					return nil, fmt.Errorf("fake error")
				},
				po: &kubelet.Pod{},
			},
			fields: fields{
				clusterAgentEnabled: false,
				dcaClient:           &FakeDCAClient{},
			},
			want:    nil,
			wantErr: true,
		},

		{
			name: "clusterAgentEnabled enable, but old version",
			args: args{
				getPodMetaDataFromAPIServerFunc: func(string, string, string) ([]string, error) {
					return []string{"foo=bar"}, nil
				},
				po: &kubelet.Pod{},
			},
			fields: fields{
				clusterAgentEnabled: true,
				dcaClient: &FakeDCAClient{
					LocalVersion:            version.Version{Major: 1, Minor: 2},
					KubernetesMetadataNames: []string{"foo=bar"},
				},
			},
			want:    []string{"foo=bar"},
			wantErr: false,
		},

		{
			name: "clusterAgentEnabled enable, but old version",
			args: args{
				getPodMetaDataFromAPIServerFunc: func(string, string, string) ([]string, error) {
					return []string{"foo=bar"}, nil
				},
				po: &kubelet.Pod{},
			},
			fields: fields{
				clusterAgentEnabled: true,
				dcaClient:           &clusteragent.DCAClient{},
			},
			want:    []string{"foo=bar"},
			wantErr: false,
		},

		{
			name: "clusterAgentEnabled enable, but old version, DCS return error",
			args: args{
				getPodMetaDataFromAPIServerFunc: func(string, string, string) ([]string, error) {
					return []string{"foo=bar"}, nil
				},
				po: &kubelet.Pod{},
			},
			fields: fields{
				clusterAgentEnabled: true,
				dcaClient: &FakeDCAClient{
					LocalVersion:               version.Version{Major: 1, Minor: 2},
					KubernetesMetadataNamesErr: fmt.Errorf("fake error"),
				},
			},
			want:    nil,
			wantErr: true,
		},

		{
			name: "clusterAgentEnabled enable with new version",
			args: args{
				getPodMetaDataFromAPIServerFunc: func(string, string, string) ([]string, error) {
					return []string{"foo=bar"}, nil
				},
				po: &kubelet.Pod{Metadata: kubelet.PodMetadata{
					Namespace: "test",
					Name:      "pod-bar",
				}},
				metadataByNsPods: apiv1.NamespacesPodsStringsSet{
					"test": apiv1.MapStringSet{
						"pod-bar": sets.New("foo=bar"),
					},
				},
			},
			fields: fields{
				clusterAgentEnabled: true,
				dcaClient: &FakeDCAClient{
					LocalVersion: version.Version{Major: 1, Minor: 3},
				},
			},
			want:    []string{"foo=bar"},
			wantErr: false,
		},
		{
			name: "clusterAgentEnabled enable with new version (error case, pod not exist)",
			args: args{
				getPodMetaDataFromAPIServerFunc: func(string, string, string) ([]string, error) {
					return []string{"foo=bar"}, nil
				},
				po: &kubelet.Pod{Metadata: kubelet.PodMetadata{
					Namespace: "test",
					Name:      "pod-bar",
				}},
				metadataByNsPods: apiv1.NamespacesPodsStringsSet{
					"test": apiv1.MapStringSet{},
				},
			},
			fields: fields{
				clusterAgentEnabled: true,
				dcaClient: &FakeDCAClient{
					LocalVersion: version.Version{Major: 1, Minor: 3},
				},
			},
			want:    nil,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &collector{
				dcaClient:  tt.fields.dcaClient,
				dcaEnabled: tt.fields.clusterAgentEnabled,
			}
			got, err := c.getMetadata(tt.args.getPodMetaDataFromAPIServerFunc, tt.args.metadataByNsPods, tt.args.po)
			if (err != nil) != tt.wantErr {
				t.Errorf("KubeMetadataCollector.getMetadaNames() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("KubeMetadataCollector.getMetadaNames() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestKubeMetadataCollector_getNamespaceMetadata(t *testing.T) {
	type fields struct {
		dcaClient           clusteragent.DCAClientInterface
		clusterAgentEnabled bool
	}

	tests := []struct {
		name                       string
		fields                     fields
		namespaceAnnotationsAsTags map[string]string
		namespaceLabelsAsTags      map[string]string
		want                       *workloadmeta.EntityMeta
		wantErr                    bool
	}{
		{
			name:    "no namespace annotations as tags and no namespace labels as tags",
			want:    nil,
			wantErr: false,
		},
		{
			name: "cluster agent not enabled",
			fields: fields{
				clusterAgentEnabled: false,
				dcaClient:           &FakeDCAClient{},
			},
			namespaceAnnotationsAsTags: map[string]string{
				"annot-key": "annot-tag",
			},
			namespaceLabelsAsTags: map[string]string{
				"label-key": "label-tag",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "cluster agent enabled",
			fields: fields{
				clusterAgentEnabled: true,
				dcaClient: &FakeDCAClient{
					LocalVersion: version.Version{Major: 1, Minor: 12},
					NamespaceMetadata: clusteragent.Metadata{
						Annotations: map[string]string{
							"annot-key": "value",
						},
						Labels: map[string]string{
							"label-key": "value",
						},
					},
				},
			},
			namespaceAnnotationsAsTags: map[string]string{
				"annot-key": "annot-tag",
			},
			namespaceLabelsAsTags: map[string]string{
				"label-key": "label-tag",
			},
			want: &workloadmeta.EntityMeta{
				Labels:      map[string]string{"label-key": "value"},
				Annotations: map[string]string{"annot-key": "value"},
			},
			wantErr: false,
		},
		{
			name: "cluster agent enabled and failed to get namespace metadata",
			fields: fields{
				clusterAgentEnabled: true,
				dcaClient: &FakeDCAClient{
					LocalVersion:         version.Version{Major: 1, Minor: 12},
					NamespaceMetadataErr: errors.New("failed to get namespace metadata"),
				},
			},
			namespaceAnnotationsAsTags: map[string]string{
				"key": "tag",
			},
			namespaceLabelsAsTags: map[string]string{
				"key": "tag",
			},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &collector{
				dcaClient:                   tt.fields.dcaClient,
				dcaEnabled:                  tt.fields.clusterAgentEnabled,
				collectNamespaceAnnotations: len(tt.namespaceAnnotationsAsTags) > 0,
				collectNamespaceLabels:      len(tt.namespaceLabelsAsTags) > 0,
			}

			metadata, err := c.getNamespaceMetadata("foo")
			assert.True(t, (err != nil) == tt.wantErr)
			assert.EqualValues(&testing.T{}, tt.want, metadata)
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
	cache.Cache.Set("KubeletPodListCacheKey", podsCache, 2*time.Second)
	kubeUtilFake := &kubelet.KubeUtil{}

	type fields struct {
		kubeUtil                    *kubelet.KubeUtil
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
				kubeUtil:               kubeUtilFake,
				dcaEnabled:             true,
				collectNamespaceLabels: true,
				dcaClient: &FakeDCAClient{
					LocalVersion:            version.Version{Major: 1, Minor: 3},
					KubernetesMetadataNames: []string{"svc1", "svc2"},
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
			},
			wantErr: false,
		},
		{
			name: "clusterAgentEnabled enabled, cluster-agent version < 7.55, ns labels enabled, ns annotations enabled",
			args: args{
				pods: pods,
			},
			fields: fields{
				kubeUtil:                    kubeUtilFake,
				dcaEnabled:                  true,
				collectNamespaceLabels:      true,
				collectNamespaceAnnotations: true,
				dcaClient: &FakeDCAClient{
					LocalVersion:            version.Version{Major: 1, Minor: 3},
					KubernetesMetadataNames: []string{"svc1", "svc2"},
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
			},
			wantErr: false,
		},
		{
			name: "clusterAgentEnabled enabled, cluster-agent version >= 7.55, ns labels collection enabled, ns annotations collection enabled",
			args: args{
				pods: pods,
			},
			fields: fields{
				kubeUtil:                    kubeUtilFake,
				dcaEnabled:                  true,
				collectNamespaceLabels:      true,
				collectNamespaceAnnotations: true,
				dcaClient: &FakeDCAClient{
					LocalVersion:            version.Version{Major: 7, Minor: 55},
					KubernetesMetadataNames: []string{"svc1", "svc2"},
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
			},
			wantErr: false,
		},
		{
			name: "clusterAgentEnabled enabled, cluster-agent version >= 7.55, ns labels collection enabled, ns annotations collection disabled",
			args: args{
				pods: pods,
			},
			fields: fields{
				kubeUtil:                    kubeUtilFake,
				dcaEnabled:                  true,
				collectNamespaceLabels:      true,
				collectNamespaceAnnotations: false,
				dcaClient: &FakeDCAClient{
					LocalVersion:            version.Version{Major: 7, Minor: 55},
					KubernetesMetadataNames: []string{"svc1", "svc2"},
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
			},
			wantErr: false,
		},
		{
			name: "clusterAgentEnabled enabled, ns labels disabled",
			args: args{
				pods: pods,
			},
			fields: fields{
				kubeUtil:               kubeUtilFake,
				dcaEnabled:             true,
				collectNamespaceLabels: false,
				dcaClient: &FakeDCAClient{
					LocalVersion:            version.Version{Major: 1, Minor: 3},
					KubernetesMetadataNames: []string{"svc1", "svc2"},
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
			},
			wantErr: false,
		},
		{
			name: "clusterAgentEnabled enabled, but client init failed",
			args: args{
				pods: pods,
			},
			fields: fields{
				kubeUtil:   kubeUtilFake,
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
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &collector{
				kubeUtil:                    tt.fields.kubeUtil,
				apiClient:                   tt.fields.apiClient,
				dcaClient:                   tt.fields.dcaClient,
				lastUpdate:                  tt.fields.lastUpdate,
				updateFreq:                  tt.fields.updateFreq,
				dcaEnabled:                  tt.fields.dcaEnabled,
				collectNamespaceLabels:      tt.fields.collectNamespaceLabels,
				collectNamespaceAnnotations: tt.fields.collectNamespaceAnnotations,
				seen:                        make(map[workloadmeta.EntityID]struct{}),
			}

			got, err := c.parsePods(context.TODO(), tt.args.pods, make(map[workloadmeta.EntityID]struct{}))
			assert.True(t, (err != nil) == tt.wantErr)
			assert.ElementsMatch(t, tt.want, got)
		})
	}
}
