// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver,kubelet

package collectors

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	apiv1 "github.com/DataDog/datadog-agent/pkg/clusteragent/api/v1"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/version"
	"k8s.io/apimachinery/pkg/util/sets"
)

type FakeDCAClient struct {
	LocalVersion                 version.Version
	LocalClusterAgentAPIEndpoint string

	VersionErr error

	NodeLabel    map[string]string
	NodeLabelErr error

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
}

func (f *FakeDCAClient) Version() version.Version {
	return f.LocalVersion
}
func (f *FakeDCAClient) ClusterAgentAPIEndpoint() string {
	return f.LocalClusterAgentAPIEndpoint
}
func (f *FakeDCAClient) GetVersion() (version.Version, error) {
	return f.LocalVersion, f.VersionErr
}
func (f *FakeDCAClient) GetNodeLabels(nodeName string) (map[string]string, error) {
	return f.NodeLabel, f.NodeLabelErr
}
func (f *FakeDCAClient) GetPodsMetadataForNode(nodeName string) (apiv1.NamespacesPodsStringsSet, error) {
	return f.PodMetadataForNode, f.PodMetadataForNodeErr
}
func (f *FakeDCAClient) GetKubernetesMetadataNames(nodeName, ns, podName string) ([]string, error) {
	return f.KubernetesMetadataNames, f.KubernetesMetadataNamesErr
}

func (f *FakeDCAClient) PostClusterCheckStatus(nodeName string, status types.NodeStatus) (types.StatusResponse, error) {
	return f.ClusterCheckStatus, f.ClusterCheckStatusErr
}

func (f *FakeDCAClient) GetClusterCheckConfigs(nodeName string) (types.ConfigResponse, error) {
	return f.ClusterCheckConfigs, f.ClusterCheckConfigsErr
}

func (f *FakeDCAClient) GetEndpointsCheckConfigs(nodeName string) (types.ConfigResponse, error) {
	return f.EndpointsCheckConfigs, f.EndpointsCheckConfigsErr
}

func (f *FakeDCAClient) GetAllCFAppsMetadata() (map[string][]string, error) {
	panic("implement me")
}

func TestKubeMetadataCollector_getMetadaNames(t *testing.T) {
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
						"pod-bar": sets.NewString("foo=bar"),
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
			c := &KubeMetadataCollector{
				dcaClient:           tt.fields.dcaClient,
				clusterAgentEnabled: tt.fields.clusterAgentEnabled,
			}
			got, err := c.getMetadaNames(tt.args.getPodMetaDataFromAPIServerFunc, tt.args.metadataByNsPods, tt.args.po)
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

func TestKubeMetadataCollector_getTagInfos(t *testing.T) {
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
		kubeUtil            *kubelet.KubeUtil
		apiClient           *apiserver.APIClient
		infoOut             chan<- []*TagInfo
		dcaClient           clusteragent.DCAClientInterface
		lastUpdate          time.Time
		updateFreq          time.Duration
		clusterAgentEnabled bool
	}
	type args struct {
		pods []*kubelet.Pod
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   []*TagInfo
	}{
		{
			name: "clusterAgentEnabled enable cluster-agent 1.3.x >=",
			args: args{
				pods: pods,
			},
			fields: fields{
				kubeUtil:            kubeUtilFake,
				clusterAgentEnabled: true,
				dcaClient: &FakeDCAClient{
					LocalVersion:            version.Version{Major: 1, Minor: 3},
					KubernetesMetadataNames: []string{"svc1", "svc2"},
				},
			},
			want: []*TagInfo{
				{
					Source:               kubeMetadataCollectorName,
					Entity:               kubelet.PodUIDToTaggerEntityName("foouid"),
					HighCardTags:         []string{},
					OrchestratorCardTags: []string{},
					LowCardTags: []string{
						"kube_service:svc1",
						"kube_service:svc2",
					},
				},
			},
		},
		{
			name: "clusterAgentEnabled enable but client init failed",
			args: args{
				pods: pods,
			},
			fields: fields{
				kubeUtil:            kubeUtilFake,
				clusterAgentEnabled: true,
				dcaClient:           &FakeDCAClient{},
			},
			want: []*TagInfo{
				{
					Source:               kubeMetadataCollectorName,
					Entity:               kubelet.PodUIDToTaggerEntityName("foouid"),
					HighCardTags:         []string{},
					OrchestratorCardTags: []string{},
					LowCardTags:          []string{},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &KubeMetadataCollector{
				kubeUtil:            tt.fields.kubeUtil,
				apiClient:           tt.fields.apiClient,
				infoOut:             tt.fields.infoOut,
				dcaClient:           tt.fields.dcaClient,
				lastUpdate:          tt.fields.lastUpdate,
				updateFreq:          tt.fields.updateFreq,
				clusterAgentEnabled: tt.fields.clusterAgentEnabled,
			}

			got := c.getTagInfos(tt.args.pods)
			assertTagInfoListEqual(t, tt.want, got)
		})
	}
}
