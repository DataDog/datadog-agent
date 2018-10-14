// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build kubeapiserver

package hpa

import (
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/stretchr/testify/assert"
	autoscalingv2 "k8s.io/api/autoscaling/v2beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/custommetrics"
)

func TestDiffAutoscalter(t *testing.T) {
	testCases := map[string]struct {
		hpaOld   *autoscalingv2.HorizontalPodAutoscaler
		hpaNew   *autoscalingv2.HorizontalPodAutoscaler
		expected bool
	}{
		"No-Op": {
			&autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"kubectl.kubernetes.io/last-applied-configuration": `
						"apiVersion":"autoscaling/v2beta1",
						"kind":"HorizontalPodAutoscaler",
						"metadata":{
							"annotations":{},
							"name":"nginxext1",
							"namespace":
							"default"
						},
						"spec":{
							"maxReplicas":5,
							"metrics":[{
								"external":{
									"metricName":"nginx.net.request_per_s",
									"metricSelector":{
										"matchLabels":{
											"cluster-name":"sic-kenafeh",
											"kube_container_name":"nginx"
										}
								},
								"targetValue":5},
								"type":"External"
							}],
							"minReplicas":1,
							"scaleTargetRef":{
								"apiVersion":"apps/v1",
								"kind":"Deployment",
								"name":"nginx"
							}
						}"`,
					},
				},
				Status: autoscalingv2.HorizontalPodAutoscalerStatus{
					CurrentReplicas: 27,
				},
			},
			&autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"kubectl.kubernetes.io/last-applied-configuration": `
						"apiVersion":"autoscaling/v2beta1",
						"kind":"HorizontalPodAutoscaler",
						"metadata":{
							"annotations":{},
							"name":"nginxext1",
							"namespace":
							"default"
						},
						"spec":{
							"maxReplicas":5,
							"metrics":[{
								"external":{
									"metricName":"nginx.net.request_per_s",
									"metricSelector":{
										"matchLabels":{
											"cluster-name":"sic-kenafeh",
											"kube_container_name":"nginx"
										}
								},
								"targetValue":5},
								"type":"External"
							}],
							"minReplicas":1,
							"scaleTargetRef":{
								"apiVersion":"apps/v1",
								"kind":"Deployment",
								"name":"nginx"
							}
						}"`,
					},
				},
				Status: autoscalingv2.HorizontalPodAutoscalerStatus{
					CurrentReplicas: 12,
				},
			},
			false,
		},
		"Updated": {
			&autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"kubectl.kubernetes.io/last-applied-configuration": `
						"apiVersion":"autoscaling/v2beta1",
						"kind":"HorizontalPodAutoscaler",
						"metadata":{
							"annotations":{},
							"name":"nginxext1",
							"namespace":
							"default"
						},
						"spec":{
							"maxReplicas":5,
							"metrics":[{
								"external":{
									"metricName":"nginx.net.request_per_s",
									"metricSelector":{
										"matchLabels":{
											"cluster-name":"sic-kenafeh",
											"kube_container_name":"nginx"
										}
								},
								"targetValue":5},
								"type":"External"
							}],
							"minReplicas":1,
							"scaleTargetRef":{
								"apiVersion":"apps/v1",
								"kind":"Deployment",
								"name":"nginx"
							}
						}"`,
					},
				},
			},
			&autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"kubectl.kubernetes.io/last-applied-configuration": `
						"apiVersion":"autoscaling/v2beta1",
						"kind":"HorizontalPodAutoscaler",
						"metadata":{
							"annotations":{},
							"name":"nginxext1",
							"namespace":
							"default"
						},
						"spec":{
							"maxReplicas":5,
							"metrics":[{
								"external":{
									"metricName":"nginx.net.request_per_s",
									"metricSelector":{
										"matchLabels":{
											"kube_container_name":"apache"
										}
								},
								"targetValue":5},
								"type":"External"
							}],
							"minReplicas":1,
							"scaleTargetRef":{
								"apiVersion":"apps/v1",
								"kind":"Deployment",
								"name":"nginx"
							}
						}"`,
					},
				},
			},
			true,
		},
	}
	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			val := AutoscalerMetricsUpdate(testCase.hpaNew, testCase.hpaOld)
			assert.Equal(t, testCase.expected, val)
		})
	}
}

func TestInspect(t *testing.T) {
	metricName := "requests_per_s"

	testCases := map[string]struct {
		hpa      *autoscalingv2.HorizontalPodAutoscaler
		expected []custommetrics.ExternalMetricValue
	}{
		"with external metrics": {
			&autoscalingv2.HorizontalPodAutoscaler{
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					Metrics: []autoscalingv2.MetricSpec{
						{
							Type: autoscalingv2.ExternalMetricSourceType,
							External: &autoscalingv2.ExternalMetricSource{
								MetricName: metricName,
								MetricSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										"dcos_version": "1.9.4",
									},
								},
							},
						},
						{
							Type: autoscalingv2.ExternalMetricSourceType,
							External: &autoscalingv2.ExternalMetricSource{
								MetricName: metricName,
								MetricSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										"dcos_version": "2.1.9",
									},
								},
							},
						},
					},
				},
			},
			[]custommetrics.ExternalMetricValue{
				{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"dcos_version": "1.9.4"},
					Timestamp:  0,
					Value:      0,
					Valid:      false,
				},
				{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"dcos_version": "2.1.9"},
					Timestamp:  0,
					Value:      0,
					Valid:      false,
				},
			},
		},
		"no external metrics": {
			&autoscalingv2.HorizontalPodAutoscaler{
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					Metrics: []autoscalingv2.MetricSpec{
						{
							Type: autoscalingv2.PodsMetricSourceType,
							Pods: &autoscalingv2.PodsMetricSource{
								MetricName:         metricName,
								TargetAverageValue: resource.MustParse("12"),
							},
						},
					},
				},
			},
			[]custommetrics.ExternalMetricValue{},
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			got := Inspect(testCase.hpa)
			assert.ElementsMatch(t, testCase.expected, got)
		})
	}
}

func TestDiffExternalMetrics(t *testing.T) {
	testCases := map[string]struct {
		lhs      []*autoscalingv2.HorizontalPodAutoscaler
		rhs      []custommetrics.ExternalMetricValue
		expected []custommetrics.ExternalMetricValue
	}{
		"delete invalid metric": {

			[]*autoscalingv2.HorizontalPodAutoscaler{
				{
					ObjectMeta: metav1.ObjectMeta{
						UID: types.UID(5),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						UID: types.UID(7),
					},
				},
			},
			[]custommetrics.ExternalMetricValue{
				{
					MetricName: "requests_per_s_one",
					Labels:     map[string]string{"foo": "bar"},
					Valid:      true,
					HPA: custommetrics.ObjectReference{
						UID: string(5),
					},
				},
				{
					MetricName: "requests_per_s_two",
					Labels:     map[string]string{"foo": "bar"},
					Valid:      false,
					HPA: custommetrics.ObjectReference{
						UID: string(6),
					},
				},
				{
					MetricName: "requests_per_s_three",
					Labels:     map[string]string{"foo": "bar"},
					Valid:      false,
					HPA: custommetrics.ObjectReference{
						UID: string(7),
					},
				},
			},
			[]custommetrics.ExternalMetricValue{
				{
					MetricName: "requests_per_s_two",
					Labels:     map[string]string{"foo": "bar"},
					Valid:      false,
					HPA: custommetrics.ObjectReference{
						UID: string(6),
					},
				},
			},
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			got := DiffExternalMetrics(testCase.lhs, testCase.rhs)
			assert.ElementsMatch(t, testCase.expected, got)
		})
	}
}
