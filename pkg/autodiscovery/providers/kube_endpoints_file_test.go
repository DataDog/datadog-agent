// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks && kubeapiserver

package providers

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestBuildConfigStore(t *testing.T) {
	tpl1 := &integration.Config{
		Name: "check1",
		AdvancedADIdentifiers: []integration.AdvancedADIdentifier{{
			KubeEndpoints: kubeNsName("ep-ns1", "ep-name1"),
		}},
	}

	tpl2 := &integration.Config{
		Name: "check2",
		AdvancedADIdentifiers: []integration.AdvancedADIdentifier{{
			KubeEndpoints: kubeNsName("ep-ns2", "ep-name2"),
		}},
	}

	tpl3 := &integration.Config{
		Name: "check3",
		AdvancedADIdentifiers: []integration.AdvancedADIdentifier{{
			KubeEndpoints: kubeNsName("ep-ns3", "ep-name3"),
			KubeService:   kubeNsName("svc-ns", "svc-name"),
		}},
	}

	tpl4 := &integration.Config{
		Name: "check4",
		AdvancedADIdentifiers: []integration.AdvancedADIdentifier{{
			KubeService: kubeNsName("svc-ns", "svc-name"),
		}},
	}

	tests := []struct {
		name      string
		templates []integration.Config
		want      map[string]*epConfig
	}{
		{
			name:      "empty",
			templates: []integration.Config{},
			want:      map[string]*epConfig{},
		},
		{
			name:      "with ep templates",
			templates: []integration.Config{*tpl1, *tpl2},
			want: map[string]*epConfig{
				"ep-ns1/ep-name1": {
					templates:     []integration.Config{*tpl1},
					ep:            nil,
					shouldCollect: false,
				},
				"ep-ns2/ep-name2": {
					templates:     []integration.Config{*tpl2},
					ep:            nil,
					shouldCollect: false,
				},
			},
		},
		{
			name:      "with ep and svc templates",
			templates: []integration.Config{*tpl3, *tpl4},
			want: map[string]*epConfig{
				"ep-ns3/ep-name3": {
					templates:     []integration.Config{*tpl3},
					ep:            nil,
					shouldCollect: false,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &KubeEndpointsFileConfigProvider{}
			p.buildConfigStore(tt.templates)

			assert.EqualValues(t, tt.want, p.store.epConfigs)
		})
	}
}

func TestStoreInsertEp(t *testing.T) {
	tpl := integration.Config{
		Name: "check1",
		AdvancedADIdentifiers: []integration.AdvancedADIdentifier{{
			KubeEndpoints: kubeNsName("ep-ns1", "ep-name1"),
		}},
	}

	ep1 := &v1.Endpoints{ObjectMeta: metav1.ObjectMeta{Name: "ep1", Namespace: "ns1"}}

	tests := []struct {
		name          string
		epConfigs     map[string]*epConfig
		ep            *v1.Endpoints
		want          bool
		wantEpConfigs map[string]*epConfig
	}{
		{
			name:          "not found",
			epConfigs:     make(map[string]*epConfig),
			ep:            ep1,
			want:          false,
			wantEpConfigs: make(map[string]*epConfig),
		},
		{
			name:          "found",
			epConfigs:     map[string]*epConfig{"ns1/ep1": {templates: []integration.Config{tpl}, ep: nil, shouldCollect: false}},
			ep:            ep1,
			want:          true,
			wantEpConfigs: map[string]*epConfig{"ns1/ep1": {templates: []integration.Config{tpl}, ep: ep1, shouldCollect: true}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &store{epConfigs: tt.epConfigs}

			got := s.insertEp(tt.ep)

			assert.Equal(t, tt.want, got)
			assert.EqualValues(t, tt.wantEpConfigs, s.epConfigs)
		})
	}
}

func TestStoreGenerateConfigs(t *testing.T) {
	tpl1 := integration.Config{
		Name: "check1",
		AdvancedADIdentifiers: []integration.AdvancedADIdentifier{{
			KubeEndpoints: kubeNsName("ep-ns1", "ep-name1"),
		}},
	}

	tpl2 := integration.Config{
		Name: "check2",
		AdvancedADIdentifiers: []integration.AdvancedADIdentifier{{
			KubeEndpoints: kubeNsName("ep-ns2", "ep-name2"),
		}},
	}

	node1, node2 := "node1", "node2"
	ep1 := &v1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{Name: "ep1", Namespace: "ns1"},
		Subsets: []v1.EndpointSubset{
			{
				Addresses: []v1.EndpointAddress{
					{IP: "10.0.0.1", NodeName: &node1, TargetRef: &v1.ObjectReference{UID: types.UID("pod-1"), Kind: "Pod"}},
					{IP: "10.0.0.2", NodeName: &node2, TargetRef: &v1.ObjectReference{UID: types.UID("pod-2"), Kind: "Pod"}},
				},
			},
		},
	}

	tests := []struct {
		name      string
		epConfigs map[string]*epConfig
		want      []integration.Config
	}{
		{
			name: "nominal case",
			epConfigs: map[string]*epConfig{
				"ns1/ep1": {templates: []integration.Config{tpl1}, ep: ep1, shouldCollect: true},
				"ns2/ep2": {templates: []integration.Config{tpl2}, ep: nil, shouldCollect: false},
			},
			want: []integration.Config{
				{
					Name:                  "check1",
					ServiceID:             "kube_endpoint_uid://ns1/ep1/10.0.0.1",
					ADIdentifiers:         []string{"kube_endpoint_uid://ns1/ep1/10.0.0.1", "kubernetes_pod://pod-1"},
					AdvancedADIdentifiers: nil,
					NodeName:              "node1",
					Provider:              "kubernetes-endpoints-file",
					ClusterCheck:          true,
				},
				{
					Name:                  "check1",
					ServiceID:             "kube_endpoint_uid://ns1/ep1/10.0.0.2",
					ADIdentifiers:         []string{"kube_endpoint_uid://ns1/ep1/10.0.0.2", "kubernetes_pod://pod-2"},
					AdvancedADIdentifiers: nil,
					NodeName:              "node2",
					Provider:              "kubernetes-endpoints-file",
					ClusterCheck:          true,
				},
			},
		},
		{
			name: "should not collect",
			epConfigs: map[string]*epConfig{
				"ns1/ep1": {templates: []integration.Config{tpl1}, ep: ep1, shouldCollect: false},
			},
			want: []integration.Config{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &store{epConfigs: tt.epConfigs}

			assert.EqualValues(t, tt.want, s.generateConfigs())
		})
	}
}
