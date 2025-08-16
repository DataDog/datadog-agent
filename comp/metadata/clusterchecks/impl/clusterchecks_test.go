// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks && test

package clusterchecksimpl

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	clustercheckhandler "github.com/DataDog/datadog-agent/comp/core/clusterchecks/def"
	clusterchecksmock "github.com/DataDog/datadog-agent/comp/core/clusterchecks/mock"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

func TestGetPayload(t *testing.T) {
	tests := []struct {
		name           string
		handlerState   types.StateResponse
		handlerError   error
		hasHandler     bool
		configEnabled  bool
		expectNil      bool
		expectChecks   []string
		expectDangling int
		expectNodes    int
	}{
		{
			name:          "config disabled",
			configEnabled: false,
			hasHandler:    true,
			expectNil:     true,
		},
		{
			name:          "no handler",
			configEnabled: true,
			hasHandler:    false,
			expectNil:     true,
		},
		{
			name:          "not leader",
			configEnabled: true,
			hasHandler:    true,
			handlerState: types.StateResponse{
				NotRunning: "currently follower",
			},
			expectNil: true,
		},
		{
			name:          "leader with checks",
			configEnabled: true,
			hasHandler:    true,
			handlerState: types.StateResponse{
				NotRunning: "", // empty means leader
				Nodes: []types.StateNodeResponse{
					{
						Name: "node-1",
						Configs: []integration.Config{
							{
								Name:         "kubernetes_state_core",
								Provider:     "cluster-checks",
								Source:       "clusterchecks",
								InitConfig:   []byte(`{}`),
								Instances:    []integration.Data{[]byte(`{"kube_state_url":"http://ksm:8080/metrics"}`)},
								ClusterCheck: true,
							},
						},
					},
					{
						Name: "node-2",
						Configs: []integration.Config{
							{
								Name:         "openmetrics",
								Provider:     "cluster-checks",
								Source:       "clusterchecks",
								InitConfig:   []byte(`{}`),
								Instances:    []integration.Data{[]byte(`{"prometheus_url":"http://metrics:9090"}`)},
								ClusterCheck: true,
							},
						},
					},
				},
				Dangling: []integration.Config{
					{
						Name:         "orchestrator",
						Provider:     "cluster-checks",
						Source:       "clusterchecks",
						InitConfig:   []byte(`{}`),
						Instances:    []integration.Data{[]byte(`{}`)},
						ClusterCheck: true,
					},
				},
			},
			expectNil:      false,
			expectChecks:   []string{"kubernetes_state_core", "openmetrics", "orchestrator"},
			expectDangling: 1,
			expectNodes:    2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mocks
			log := logmock.New(t)
			conf := configmock.New(t)
			conf.SetWithoutSource("inventories_checks_configuration_enabled", tt.configEnabled)
			conf.SetWithoutSource("cluster_name", "test-cluster")

			// Create handler option
			var handlerOpt option.Option[clustercheckhandler.Component]
			if tt.hasHandler {
				handler := clusterchecksmock.New(tt.handlerState, tt.handlerError)
				handlerOpt = option.New[clustercheckhandler.Component](handler)
			} else {
				handlerOpt = option.None[clustercheckhandler.Component]()
			}

			// Create component
			cc := &clusterChecksImpl{
				log:            log,
				conf:           conf,
				clustername:    "test-cluster",
				clusterID:      "cluster-123",
				clusterHandler: handlerOpt,
			}

			// Get payload
			payload := cc.getPayload()

			if tt.expectNil {
				assert.Nil(t, payload)
			} else {
				require.NotNil(t, payload)
				assert.Equal(t, "test-cluster", payload.Clustername)
				assert.Equal(t, "cluster-123", payload.ClusterID)

				// Check metadata
				for _, checkName := range tt.expectChecks {
					assert.Contains(t, payload.ClusterCheckMetadata, checkName)
				}

				// Check status
				assert.Equal(t, tt.expectDangling, payload.ClusterCheckStatus["dangling_count"])
				assert.Equal(t, tt.expectNodes, payload.ClusterCheckStatus["node_count"])
			}
		})
	}
}

func TestCollectClusterCheckMetadata(t *testing.T) {
	log := logmock.New(t)
	conf := configmock.New(t)

	handler := clusterchecksmock.New(types.StateResponse{
		NotRunning: "",
		Nodes: []types.StateNodeResponse{
			{
				Name: "node-1",
				Configs: []integration.Config{
					{
						Name:         "kubernetes_state_core",
						Provider:     "cluster-checks",
						Source:       "clusterchecks",
						InitConfig:   []byte(`{"init":"config"}`),
						Instances:    []integration.Data{[]byte(`{"instance":"config"}`)},
						ClusterCheck: true,
					},
				},
			},
		},
		Dangling: []integration.Config{
			{
				Name:         "openmetrics",
				Provider:     "cluster-checks",
				Source:       "clusterchecks",
				InitConfig:   []byte(`{}`),
				Instances:    []integration.Data{[]byte(`{}`)},
				ClusterCheck: true,
			},
		},
	}, nil)

	handlerOpt := option.New[clustercheckhandler.Component](handler)

	cc := &clusterChecksImpl{
		log:            log,
		conf:           conf,
		clustername:    "test-cluster",
		clusterID:      "cluster-123",
		clusterHandler: handlerOpt,
	}

	payload := &Payload{
		Clustername:          "test-cluster",
		ClusterID:            "cluster-123",
		Timestamp:            time.Now().UnixNano(),
		ClusterCheckMetadata: make(map[string][]metadata),
		ClusterCheckStatus:   make(map[string]interface{}),
	}

	cc.collectClusterCheckMetadata(payload)

	// Verify dispatched check
	assert.Contains(t, payload.ClusterCheckMetadata, "kubernetes_state_core")
	ksm := payload.ClusterCheckMetadata["kubernetes_state_core"][0]
	assert.Equal(t, "DISPATCHED", ksm["status"])
	assert.Equal(t, "node-1", ksm["node_name"])
	assert.Equal(t, "", ksm["errors"])
	assert.Equal(t, "cluster-checks", ksm["config.provider"])
	assert.Equal(t, "clusterchecks", ksm["config.source"])
	assert.Equal(t, `{"init":"config"}`, ksm["init_config"])
	assert.Equal(t, `{"instance":"config"}`, ksm["instance_config"])

	// Verify dangling check
	assert.Contains(t, payload.ClusterCheckMetadata, "openmetrics")
	om := payload.ClusterCheckMetadata["openmetrics"][0]
	assert.Equal(t, "DANGLING", om["status"])
	assert.Equal(t, "Check not assigned to any node", om["errors"])

	// Verify status counts
	assert.Equal(t, 1, payload.ClusterCheckStatus["dangling_count"])
	assert.Equal(t, 1, payload.ClusterCheckStatus["node_count"])
}
