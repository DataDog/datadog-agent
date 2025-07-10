// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package external

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"

	kubeAutoscaling "github.com/DataDog/agent-payload/v5/autoscaling/kubernetes"
	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"
	autoscalingv2 "k8s.io/api/autoscaling/v2"

	"google.golang.org/protobuf/types/known/structpb"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func TestRecommenderClient_GetReplicaRecommendation(t *testing.T) {
	tests := []struct {
		name            string
		dpa             model.FakePodAutoscalerInternal
		expectedRequest *kubeAutoscaling.WorkloadRecommendationRequest
		serverResponse  *kubeAutoscaling.WorkloadRecommendationReply
		expectedError   string
	}{
		{
			name: "successful recommendation with CPU objective and watermarks",
			dpa: model.FakePodAutoscalerInternal{
				Namespace: "default",
				Name:      "test-dpa",
				Spec: &datadoghq.DatadogPodAutoscalerSpec{
					TargetRef: autoscalingv2.CrossVersionObjectReference{
						Kind:       "Deployment",
						Name:       "test-deployment",
						APIVersion: "apps/v1",
					},
					Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{
						{
							Type: datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType,
							PodResource: &datadoghqcommon.DatadogPodAutoscalerPodResourceObjective{
								Name: corev1.ResourceCPU,
								Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
									Utilization: pointer.Ptr[int32](80),
								},
							},
						},
					},
					Constraints: &datadoghqcommon.DatadogPodAutoscalerConstraints{
						MinReplicas: pointer.Ptr[int32](2),
						MaxReplicas: 4,
					},
				},
				CurrentReplicas: pointer.Ptr[int32](3),
				CustomRecommenderConfiguration: &model.RecommenderConfiguration{
					Endpoint: "",
					Settings: map[string]interface{}{
						"custom_setting": "value",
					},
				},
				ScalingValues: model.ScalingValues{
					Horizontal: &model.HorizontalScalingValues{
						Replicas: 3,
					},
				},
			},
			expectedRequest: &kubeAutoscaling.WorkloadRecommendationRequest{
				State: &kubeAutoscaling.WorkloadState{
					CurrentReplicas: pointer.Ptr[int32](3),
					ReadyReplicas:   pointer.Ptr[int32](1),
					DesiredReplicas: 3,
				},
				Targets: []*kubeAutoscaling.WorkloadRecommendationTarget{
					{
						Type:        "cpu",
						TargetValue: 0.80,
					},
				},
				Constraints: &kubeAutoscaling.WorkloadRecommendationConstraints{
					MinReplicas: 2,
					MaxReplicas: 4,
				},
				Settings: map[string]*structpb.Value{
					"custom_setting": structpb.NewStringValue("value"),
				},
			},
			serverResponse: &kubeAutoscaling.WorkloadRecommendationReply{
				TargetReplicas: 3,
				Timestamp:      timestamppb.New(time.Now()),
			},
		},
		{
			name: "successful recommendation with container resource objective",
			dpa: model.FakePodAutoscalerInternal{
				Namespace: "default",
				Name:      "test-dpa",
				Spec: &datadoghq.DatadogPodAutoscalerSpec{
					TargetRef: autoscalingv2.CrossVersionObjectReference{
						Kind:       "Deployment",
						Name:       "test-deployment",
						APIVersion: "apps/v1",
					},
					Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{
						{
							Type: datadoghqcommon.DatadogPodAutoscalerContainerResourceObjectiveType,
							ContainerResource: &datadoghqcommon.DatadogPodAutoscalerContainerResourceObjective{
								Name: corev1.ResourceMemory,
								Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
									Utilization: pointer.Ptr[int32](75),
								},
							},
						},
					},
				},
				CustomRecommenderConfiguration: &model.RecommenderConfiguration{
					Endpoint: "",
				},
			},
			expectedRequest: &kubeAutoscaling.WorkloadRecommendationRequest{
				Targets: []*kubeAutoscaling.WorkloadRecommendationTarget{
					{
						Type:        "memory",
						TargetValue: 0.75,
					},
				},
			},
			serverResponse: &kubeAutoscaling.WorkloadRecommendationReply{
				TargetReplicas: 5,
				Timestamp:      timestamppb.New(time.Now()),
			},
		},
		{
			name: "successful recommendation with multiple objectives",
			dpa: model.FakePodAutoscalerInternal{
				Namespace: "default",
				Name:      "test-dpa",
				Spec: &datadoghq.DatadogPodAutoscalerSpec{
					TargetRef: autoscalingv2.CrossVersionObjectReference{
						Kind:       "Deployment",
						Name:       "test-deployment",
						APIVersion: "apps/v1",
					},
					Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{
						{
							Type: datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType,
							PodResource: &datadoghqcommon.DatadogPodAutoscalerPodResourceObjective{
								Name: corev1.ResourceCPU,
								Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
									Utilization: pointer.Ptr[int32](80),
								},
							},
						},
						{
							Type: datadoghqcommon.DatadogPodAutoscalerContainerResourceObjectiveType,
							ContainerResource: &datadoghqcommon.DatadogPodAutoscalerContainerResourceObjective{
								Name: corev1.ResourceMemory,
								Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
									Utilization: pointer.Ptr[int32](75),
								},
							},
						},
					},
				},
				CustomRecommenderConfiguration: &model.RecommenderConfiguration{
					Endpoint: "",
				},
			},
			expectedRequest: &kubeAutoscaling.WorkloadRecommendationRequest{
				Targets: []*kubeAutoscaling.WorkloadRecommendationTarget{
					{
						Type:        "cpu",
						TargetValue: 0.80,
					},
					{
						Type:        "memory",
						TargetValue: 0.75,
					},
				},
			},
			serverResponse: &kubeAutoscaling.WorkloadRecommendationReply{
				TargetReplicas: 5,
				Timestamp:      timestamppb.New(time.Now()),
			},
		},
		{
			name: "missing recommender config",
			dpa: model.FakePodAutoscalerInternal{
				Namespace: "default",
				Name:      "test-dpa",
				Spec: &datadoghq.DatadogPodAutoscalerSpec{
					TargetRef: autoscalingv2.CrossVersionObjectReference{
						Kind:       "Deployment",
						Name:       "test-deployment",
						APIVersion: "apps/v1",
					},
					Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{
						{
							Type: datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType,
							PodResource: &datadoghqcommon.DatadogPodAutoscalerPodResourceObjective{
								Name: corev1.ResourceCPU,
								Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
									Utilization: pointer.Ptr[int32](80),
								},
							},
						},
					},
					Constraints: &datadoghqcommon.DatadogPodAutoscalerConstraints{
						MinReplicas: pointer.Ptr[int32](2),
						MaxReplicas: 4,
					},
				},
			},
			expectedError: "external recommender spec is required",
		},
		{
			name: "invalid URL",
			dpa: model.FakePodAutoscalerInternal{
				Namespace: "default",
				Name:      "test-dpa",
				Spec: &datadoghq.DatadogPodAutoscalerSpec{
					TargetRef: autoscalingv2.CrossVersionObjectReference{
						Kind:       "Deployment",
						Name:       "test-deployment",
						APIVersion: "apps/v1",
					},
					Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{
						{
							Type: datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType,
							PodResource: &datadoghqcommon.DatadogPodAutoscalerPodResourceObjective{
								Name: corev1.ResourceCPU,
								Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
									Utilization: pointer.Ptr[int32](80),
								},
							},
						},
					},
					Constraints: &datadoghqcommon.DatadogPodAutoscalerConstraints{
						MinReplicas: pointer.Ptr[int32](2),
						MaxReplicas: 4,
					},
				},
				CustomRecommenderConfiguration: &model.RecommenderConfiguration{
					Endpoint: "http://in%val%%d",
				},
			},
			expectedError: "error parsing url: parse \"http://in%val%%d\": invalid URL escape \"%va\"",
		},
		{
			name: "invalid URL scheme",
			dpa: model.FakePodAutoscalerInternal{
				Namespace: "default",
				Name:      "test-dpa",
				Spec: &datadoghq.DatadogPodAutoscalerSpec{
					TargetRef: autoscalingv2.CrossVersionObjectReference{
						Kind:       "Deployment",
						Name:       "test-deployment",
						APIVersion: "apps/v1",
					},
					Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{
						{
							Type: datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType,
							PodResource: &datadoghqcommon.DatadogPodAutoscalerPodResourceObjective{
								Name: corev1.ResourceCPU,
								Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
									Utilization: pointer.Ptr[int32](80),
								},
							},
						},
					},
					Constraints: &datadoghqcommon.DatadogPodAutoscalerConstraints{
						MinReplicas: pointer.Ptr[int32](2),
						MaxReplicas: 4,
					},
				},
				CustomRecommenderConfiguration: &model.RecommenderConfiguration{
					Endpoint: "ftp://invalid-scheme",
				},
			},
			expectedError: "only http and https schemes are supported",
		},
		{
			name: "http call returns unexpected response code",
			dpa: model.FakePodAutoscalerInternal{
				Namespace: "default",
				Name:      "test-dpa",
				Spec: &datadoghq.DatadogPodAutoscalerSpec{
					TargetRef: autoscalingv2.CrossVersionObjectReference{
						Kind:       "Deployment",
						Name:       "test-deployment",
						APIVersion: "apps/v1",
					},
					Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{
						{
							Type: datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType,
							PodResource: &datadoghqcommon.DatadogPodAutoscalerPodResourceObjective{
								Name: corev1.ResourceCPU,
								Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
									Utilization: pointer.Ptr[int32](80),
								},
							},
						},
					},
				},
				CustomRecommenderConfiguration: &model.RecommenderConfiguration{
					Endpoint: "",
					Settings: map[string]interface{}{
						"custom_setting": "value",
					},
				},
			},
			serverResponse: &kubeAutoscaling.WorkloadRecommendationReply{
				Error: &kubeAutoscaling.Error{
					Code:    pointer.Ptr[int32](404),
					Message: "Not Found",
				},
			},
			expectedError: "unexpected response code: 404",
		},
		{
			name: "recommender returns error",
			dpa: model.FakePodAutoscalerInternal{
				Namespace: "default",
				Name:      "test-dpa",
				Spec: &datadoghq.DatadogPodAutoscalerSpec{
					TargetRef: autoscalingv2.CrossVersionObjectReference{
						Kind:       "Deployment",
						Name:       "test-deployment",
						APIVersion: "apps/v1",
					},
					Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{
						{
							Type: datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType,
							PodResource: &datadoghqcommon.DatadogPodAutoscalerPodResourceObjective{
								Name: corev1.ResourceCPU,
								Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
									Utilization: pointer.Ptr[int32](80),
								},
							},
						},
					},
				},
				CustomRecommenderConfiguration: &model.RecommenderConfiguration{
					Endpoint: "",
					Settings: map[string]interface{}{
						"custom_setting": "value",
					},
				},
			},
			expectedError: "error from recommender: 200 Some random error",
			serverResponse: &kubeAutoscaling.WorkloadRecommendationReply{
				Error: &kubeAutoscaling.Error{
					Code:    pointer.Ptr[int32](200),
					Message: "Some random error",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request method and headers
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
				assert.Equal(t, "datadog-cluster-agent", r.Header.Get("User-Agent"))

				// If we expect a specific request, verify it
				if tt.expectedRequest != nil {
					var actualRequest kubeAutoscaling.WorkloadRecommendationRequest
					body, err := io.ReadAll(r.Body)
					assert.NoError(t, err)
					err = protojson.Unmarshal(body, &actualRequest)
					assert.NoError(t, err)

					// Compare relevant fields
					if tt.expectedRequest.State != nil {
						assert.Equal(t, tt.expectedRequest.State.CurrentReplicas, actualRequest.State.CurrentReplicas)
						assert.Equal(t, tt.expectedRequest.State.DesiredReplicas, actualRequest.State.DesiredReplicas)
						assert.Equal(t, tt.expectedRequest.State.ReadyReplicas, actualRequest.State.ReadyReplicas)
					}
					if tt.expectedRequest.Targets != nil {
						assert.Equal(t, tt.expectedRequest.Targets[0].Type, actualRequest.Targets[0].Type)
						assert.Equal(t, tt.expectedRequest.Targets[0].LowerBound, actualRequest.Targets[0].LowerBound, 0.01)
						assert.Equal(t, tt.expectedRequest.Targets[0].UpperBound, actualRequest.Targets[0].UpperBound, 0.01)
					}
					if tt.expectedRequest.Constraints != nil {
						assert.Equal(t, tt.expectedRequest.Constraints.MinReplicas, actualRequest.Constraints.MinReplicas)
						assert.Equal(t, tt.expectedRequest.Constraints.MaxReplicas, actualRequest.Constraints.MaxReplicas)
					}
					if tt.expectedRequest.Settings != nil {
						// Compare settings values individually
						for k, expectedVal := range tt.expectedRequest.Settings {
							actualVal, ok := actualRequest.Settings[k]
							assert.True(t, ok, "Missing expected setting %s", k)
							assert.Equal(t, expectedVal.GetStringValue(), actualVal.GetStringValue(), "Setting %s value mismatch", k)
						}
					}
				}

				payload, err := protojson.Marshal(tt.serverResponse)
				if err != nil {
					t.Errorf("Failed to marshal response: %v", err)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}

				w.Header().Set("Content-Type", "application/json")
				if tt.serverResponse.Error != nil {
					w.WriteHeader(int(*tt.serverResponse.Error.Code))
				} else {
					w.WriteHeader(http.StatusOK)
				}
				w.Write(payload)
			}))
			defer server.Close()

			// The endpoint is only set for test cases that expect an error
			if tt.dpa.CustomRecommenderConfiguration != nil && tt.dpa.CustomRecommenderConfiguration.Endpoint == "" {
				tt.dpa.CustomRecommenderConfiguration.Endpoint = server.URL
			}

			pw := workload.NewPodWatcher(nil, nil)
			pw.HandleEvent(newFakeWLMPodEvent(tt.dpa.Namespace, tt.dpa.Spec.TargetRef.Name, "pod1", []string{"container-name1"}))

			client := newRecommenderClient(pw)
			client.client = server.Client()

			result, err := client.GetReplicaRecommendation(context.Background(), "test-cluster", tt.dpa.Build())

			if tt.expectedError != "" {
				assert.EqualError(t, err, tt.expectedError)
				assert.Nil(t, result)
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, result)
			assert.Equal(t, tt.serverResponse.TargetReplicas, result.Replicas)
			assert.Equal(t, tt.serverResponse.Timestamp.AsTime(), result.Timestamp)
			assert.Equal(t, datadoghqcommon.DatadogPodAutoscalerExternalValueSource, result.Source)
		})
	}
}
