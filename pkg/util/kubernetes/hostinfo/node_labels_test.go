// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package hostinfo

import (
	"context"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
	k "github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"

	"github.com/stretchr/testify/mock"
)

type kubeUtilMock struct {
	k.KubeUtilInterface
	mock.Mock
}

func (m *kubeUtilMock) GetNodename(ctx context.Context) (string, error) {
	args := m.Called()
	return args.String(0), args.Error(1)
}

func TestNodeInfo_GetNodeClusterNameLabel(t *testing.T) {
	tests := []struct {
		name               string
		ctx                context.Context
		currentClusterName string
		mockClientFunc     func(*kubeUtilMock)
		mockConfFunc       func(conf *config.MockConfig)
		nodeLabels         map[string]string
		want               string
		wantErr            bool
	}{
		{
			name: "cluster-name not set",
			mockClientFunc: func(ku *kubeUtilMock) {
				ku.On("GetNodename").Return("node-name", nil)
			},
			nodeLabels: map[string]string{},
			ctx:        context.Background(),
			want:       "",
			wantErr:    false,
		},
		{
			name: "cluster-name label set",
			mockClientFunc: func(ku *kubeUtilMock) {
				ku.On("GetNodename").Return("node-name", nil)
			},
			nodeLabels: map[string]string{
				"ad.datadoghq.com/cluster-name": "foo",
			},
			ctx:     context.Background(),
			want:    "foo",
			wantErr: false,
		},
		{
			name: "cluster-name custom label set",
			mockClientFunc: func(ku *kubeUtilMock) {
				ku.On("GetNodename").Return("node-name", nil)
			},
			mockConfFunc: func(conf *config.MockConfig) {
				conf.Set("kubernetes_node_label_as_cluster_name", "custom-label")
			},
			nodeLabels: map[string]string{
				"custom-label": "bar",
			},
			ctx:     context.Background(),
			want:    "bar",
			wantErr: false,
		},
		{
			name: "cluster-name if custom label set, not look at default labels",
			mockClientFunc: func(ku *kubeUtilMock) {
				ku.On("GetNodename").Return("node-name", nil)
			},
			mockConfFunc: func(conf *config.MockConfig) {
				conf.Set("kubernetes_node_label_as_cluster_name", "custom-label")
			},
			nodeLabels: map[string]string{
				"ad.datadoghq.com/cluster-name": "foo",
				"custom-label":                  "bar",
			},
			ctx:     context.Background(),
			want:    "bar",
			wantErr: false,
		},
		{
			name: "a clusterName already discover, EKS label should not override the current clusterName",
			mockClientFunc: func(ku *kubeUtilMock) {
				ku.On("GetNodename").Return("node-name", nil)
			},
			currentClusterName: "bar",
			nodeLabels: map[string]string{
				"alpha.eksctl.io/cluster-name": "foo",
			},
			ctx:     context.Background(),
			want:    "bar",
			wantErr: false,
		},
		{
			name: "a clusterName already discover, AD label should override the current clusterName",
			mockClientFunc: func(ku *kubeUtilMock) {
				ku.On("GetNodename").Return("node-name", nil)
			},
			currentClusterName: "bar",
			nodeLabels: map[string]string{
				"ad.datadoghq.com/cluster-name": "foo",
			},
			ctx:     context.Background(),
			want:    "foo",
			wantErr: false,
		},
		{
			name: "cluster-name if custom label set, custom label should override the current clusterName",
			mockClientFunc: func(ku *kubeUtilMock) {
				ku.On("GetNodename").Return("node-name", nil)
			},
			mockConfFunc: func(conf *config.MockConfig) {
				conf.Set("kubernetes_node_label_as_cluster_name", "custom-label")
			},
			currentClusterName: "bar",
			nodeLabels: map[string]string{
				"custom-label": "foo",
			},
			ctx:     context.Background(),
			want:    "foo",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ku := &kubeUtilMock{}
			if tt.mockClientFunc != nil {
				tt.mockClientFunc(ku)
			}

			mockConfig := config.Mock(t)
			if tt.mockConfFunc != nil {
				tt.mockConfFunc(mockConfig)
			}

			nodeInfo := &NodeInfo{
				client:              ku,
				getClusterAgentFunc: clusteragent.GetClusterAgentClient,
				apiserverNodeLabelsFunc: func(ctx context.Context, nodeName string) (map[string]string, error) {
					return tt.nodeLabels, nil
				},
			}

			got, err := nodeInfo.GetNodeClusterNameLabel(tt.ctx, tt.currentClusterName)
			if (err != nil) != tt.wantErr {
				t.Errorf("NodeInfo.GetNodeClusterNameLabel() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("NodeInfo.GetNodeClusterNameLabel() = %v, want %v", got, tt.want)
			}

			ku.AssertExpectations(t)
		})
	}
}
