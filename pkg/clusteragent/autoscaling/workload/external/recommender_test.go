// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package external

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"
	autoscalingv2 "k8s.io/api/autoscaling/v2"

	kubeAutoscaling "github.com/DataDog/agent-payload/v5/autoscaling/kubernetes"
	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func TestProcess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// setup podwatcher
	pw := workload.NewPodWatcher(nil, nil)
	pw.HandleEvent(newFakeWLMPodEvent("default", "test-deployment", "pod1", []string{"container-name1", "container-name2"}))

	recommendationReplicas := int32(3)
	recommendationTimestamp := time.Now()
	// setup server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method and headers
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "datadog-cluster-agent", r.Header.Get("User-Agent"))

		serverResponse := &kubeAutoscaling.WorkloadRecommendationReply{
			TargetReplicas: recommendationReplicas,
			Timestamp:      timestamppb.New(recommendationTimestamp),
		}

		payload, err := protojson.Marshal(serverResponse)
		if err != nil {
			t.Errorf("Failed to marshal response: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(payload)
	}))
	defer server.Close()

	dpaExternal := model.FakePodAutoscalerInternal{
		Namespace: "default",
		Name:      "autoscaler1",
		Spec: &datadoghq.DatadogPodAutoscalerSpec{
			TargetRef: autoscalingv2.CrossVersionObjectReference{
				Kind:       "Deployment",
				APIVersion: "apps/v1",
				Name:       "test-deployment",
			},
			Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{
				{
					Type: datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType,
					PodResource: &datadoghqcommon.DatadogPodAutoscalerPodResourceObjective{
						Name: "cpu",
						Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
							Type:        datadoghqcommon.DatadogPodAutoscalerUtilizationObjectiveValueType,
							Utilization: pointer.Ptr(int32(80)),
						},
					},
				},
			},
		},
		CustomRecommenderConfiguration: &model.RecommenderConfiguration{
			Endpoint: server.URL,
			Settings: map[string]interface{}{
				"custom_setting": "value",
			},
		},
	}.Build()

	dpaLocal := model.FakePodAutoscalerInternal{
		Namespace: "default",
		Name:      "autoscaler2",
		Spec: &datadoghq.DatadogPodAutoscalerSpec{
			TargetRef: autoscalingv2.CrossVersionObjectReference{
				Kind:       "Deployment",
				APIVersion: "apps/v1",
				Name:       "test-deployment-two",
			},
			Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{
				{
					Type: datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType,
					PodResource: &datadoghqcommon.DatadogPodAutoscalerPodResourceObjective{
						Name: "cpu",
						Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
							Type:        datadoghqcommon.DatadogPodAutoscalerUtilizationObjectiveValueType,
							Utilization: pointer.Ptr(int32(70)),
						},
					},
				},
			},
		},
	}.Build()

	// setup store
	store := autoscaling.NewStore[model.PodAutoscalerInternal]()
	store.Set("default/autoscaler1", dpaExternal, "")
	store.Set("default/autoscaler2", dpaLocal, "")

	// test
	recommender := NewRecommender(pw, store, "test-cluster")
	recommender.process(ctx)

	paiExternal, found := store.Get("default/autoscaler1")
	assert.True(t, found)
	assert.Nil(t, paiExternal.MainScalingValues().HorizontalError)
	assert.Equal(t, datadoghqcommon.DatadogPodAutoscalerExternalValueSource, paiExternal.MainScalingValues().Horizontal.Source)
	assert.Equal(t, recommendationReplicas, paiExternal.MainScalingValues().Horizontal.Replicas) // currently 1 replica, recommending scale up to 2
	assert.Equal(t, recommendationTimestamp.Unix(), paiExternal.MainScalingValues().Horizontal.Timestamp.Unix())

	// Autoscalers without external recommender annotation should not be updated
	paiLocal, found := store.Get("default/autoscaler2")
	assert.True(t, found)
	assert.Equal(t, paiLocal.MainScalingValues(), model.ScalingValues{})
}
