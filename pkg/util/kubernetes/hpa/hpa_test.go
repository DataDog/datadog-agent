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
	metricNameUpper := "ReQuesTs_Per_S"

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
					CustomTags: map[string]string{},
					Labels:     map[string]string{"dcos_version": "1.9.4"},
					Timestamp:  0,
					Value:      0,
					Valid:      false,
				},
				{
					MetricName: "requests_per_s",
					CustomTags: map[string]string{},
					Labels:     map[string]string{"dcos_version": "2.1.9"},
					Timestamp:  0,
					Value:      0,
					Valid:      false,
				},
			},
		},
		"external metrics with custom tags and aggregation": {
			&autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"custom.datadog.agg":   "sum",
						"custom.datadog.tag.1": "1.9.4",
						"custom.datadog.tag.2": "2.1.9",
					},
				},
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					Metrics: []autoscalingv2.MetricSpec{
						{
							Type: autoscalingv2.ExternalMetricSourceType,
							External: &autoscalingv2.ExternalMetricSource{
								MetricName: metricName,
								MetricSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										"dcos_version": "custom.datadog.tag.1",
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
										"dcos_version": "custom.datadog.tag.2",
									},
								},
							},
						},
					},
				},
			},
			[]custommetrics.ExternalMetricValue{
				{
					MetricName:       "requests_per_s",
					CustomAggregator: "sum",
					CustomTags:       map[string]string{"custom.datadog.tag.1": "1.9.4"},
					Labels:           map[string]string{"dcos_version": "custom.datadog.tag.1"},
					Timestamp:        0,
					Value:            0,
					Valid:            false,
				},
				{
					MetricName:       "requests_per_s",
					CustomAggregator: "sum",
					CustomTags:       map[string]string{"custom.datadog.tag.2": "2.1.9"},
					Labels:           map[string]string{"dcos_version": "custom.datadog.tag.2"},
					Timestamp:        0,
					Value:            0,
					Valid:            false,
				},
			},
		},
		"external metrics with unresolved custom tag": {
			&autoscalingv2.HorizontalPodAutoscaler{
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					Metrics: []autoscalingv2.MetricSpec{
						{
							Type: autoscalingv2.ExternalMetricSourceType,
							External: &autoscalingv2.ExternalMetricSource{
								MetricName: metricName,
								MetricSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										"dcos_version": "custom.datadog.tag.1",
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
					CustomTags: map[string]string{},
					Labels:     map[string]string{"dcos_version": "custom.datadog.tag.1"},
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
		"missing labels, still OK": {
			&autoscalingv2.HorizontalPodAutoscaler{
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					Metrics: []autoscalingv2.MetricSpec{
						{
							Type: autoscalingv2.ExternalMetricSourceType,
							External: &autoscalingv2.ExternalMetricSource{
								MetricName: "foo",
							},
						},
					},
				},
			},
			[]custommetrics.ExternalMetricValue{
				{
					MetricName: "foo",
					CustomTags: map[string]string{},
					Labels:     nil,
					Timestamp:  0,
					Value:      0,
					Valid:      false,
				},
			},
		},
		"incomplete, missing external metrics": {
			&autoscalingv2.HorizontalPodAutoscaler{
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					Metrics: []autoscalingv2.MetricSpec{
						{
							Type:     autoscalingv2.ExternalMetricSourceType,
							External: nil,
						},
					},
				},
			},
			[]custommetrics.ExternalMetricValue{},
		},
		"upper cases handled": {
			&autoscalingv2.HorizontalPodAutoscaler{
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					Metrics: []autoscalingv2.MetricSpec{
						{
							Type: autoscalingv2.ExternalMetricSourceType,
							External: &autoscalingv2.ExternalMetricSource{
								MetricName: metricNameUpper,
								MetricSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										// No need to try test upper cased labels/tags as they are not supported in Datadog
										"dcos_version": "1.9.4",
									},
								},
							},
						},
					},
				},
			},
			[]custommetrics.ExternalMetricValue{
				{
					MetricName: metricNameUpper,
					CustomTags: map[string]string{},
					Labels:     map[string]string{"dcos_version": "1.9.4"},
					Timestamp:  0,
					Value:      0,
					Valid:      false,
				},
			},
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			got := Inspect(testCase.hpa)
			assert.ElementsMatch(t, testCase.expected, got)
		})
	}
}

func makeSpec(metricName string, labels map[string]string) autoscalingv2.HorizontalPodAutoscalerSpec {
	return autoscalingv2.HorizontalPodAutoscalerSpec{
		Metrics: []autoscalingv2.MetricSpec{
			{
				Type: autoscalingv2.ExternalMetricSourceType,
				External: &autoscalingv2.ExternalMetricSource{
					MetricName: metricName,
					MetricSelector: &metav1.LabelSelector{
						MatchLabels: labels,
					},
				},
			},
		},
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
						UID:       types.UID(5),
						Namespace: "nsbar",
						Name:      "foo",
					},
					Spec: makeSpec("requests_per_s_one", map[string]string{"foo": "tagbar"}),
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						UID:       types.UID(7),
						Namespace: "zanzi",
						Name:      "bar",
					},
					Spec: makeSpec("requests_per_s_three", map[string]string{"foo": "tu"}),
				},
			},
			[]custommetrics.ExternalMetricValue{
				{
					MetricName: "requests_per_s_one",
					CustomTags: map[string]string{},
					Labels:     map[string]string{"foo": "tagbar"},
					Valid:      true,
					HPA: custommetrics.ObjectReference{
						UID:       string(5),
						Name:      "foo",
						Namespace: "nsbar",
					},
				},
				{
					MetricName: "requests_per_s_two",
					CustomTags: map[string]string{},
					Labels:     map[string]string{"foo": "dre"},
					Valid:      false,
					HPA: custommetrics.ObjectReference{
						UID:       string(6),
						Name:      "foo",
						Namespace: "baz",
					},
				},
				{
					MetricName: "requests_per_s_three",
					CustomTags: map[string]string{},
					Labels:     map[string]string{"foo": "tu"},
					Valid:      false,
					HPA: custommetrics.ObjectReference{
						UID:       string(7),
						Name:      "bar",
						Namespace: "zanzi",
					},
				},
			},
			[]custommetrics.ExternalMetricValue{
				{
					MetricName: "requests_per_s_two",
					CustomTags: map[string]string{},
					Labels:     map[string]string{"foo": "dre"},
					Valid:      false,
					HPA: custommetrics.ObjectReference{
						UID:       string(6),
						Name:      "foo",
						Namespace: "baz",
					},
				},
			},
		},
		"metric name changed": {
			[]*autoscalingv2.HorizontalPodAutoscaler{
				{
					ObjectMeta: metav1.ObjectMeta{
						UID:       types.UID(5),
						Namespace: "bar",
						Name:      "foo",
					},
					Spec: makeSpec("requests_per_s_one", map[string]string{"foo": "bar"}),
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						UID:       types.UID(7),
						Namespace: "baz",
						Name:      "foo",
					},
					Spec: makeSpec("requests_per_s_new", map[string]string{"foo": "bar"}),
				},
			},
			[]custommetrics.ExternalMetricValue{
				{
					MetricName: "requests_per_s_one",
					CustomTags: map[string]string{},
					Labels:     map[string]string{"foo": "bar"},
					Valid:      true,
					HPA: custommetrics.ObjectReference{
						UID:       string(5),
						Namespace: "bar",
						Name:      "foo",
					},
				},
				{
					MetricName: "requests_per_s_old",
					CustomTags: map[string]string{},
					Labels:     map[string]string{"foo": "bar"},
					Valid:      false,
					HPA: custommetrics.ObjectReference{
						UID:       string(7),
						Namespace: "baz",
						Name:      "foo",
					},
				},
			},
			[]custommetrics.ExternalMetricValue{
				{
					MetricName: "requests_per_s_old",
					CustomTags: map[string]string{},
					Labels:     map[string]string{"foo": "bar"},
					Valid:      false,
					HPA: custommetrics.ObjectReference{
						UID:       string(7),
						Name:      "foo",
						Namespace: "baz",
					},
				},
			},
		},
		"metric labels changed": {
			[]*autoscalingv2.HorizontalPodAutoscaler{
				{
					ObjectMeta: metav1.ObjectMeta{
						UID:       types.UID(5),
						Namespace: "bar",
						Name:      "foo",
					},
					Spec: makeSpec("requests_per_s_one", map[string]string{"foo": "bar"}),
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						UID:       types.UID(7),
						Namespace: "baz",
						Name:      "foo",
					},
					Spec: makeSpec("requests_per_s_two", map[string]string{"foo": "foobar"}),
				},
			},
			[]custommetrics.ExternalMetricValue{
				{
					MetricName: "requests_per_s_one",
					CustomTags: map[string]string{},
					Labels:     map[string]string{"foo": "bar"},
					Valid:      true,
					HPA: custommetrics.ObjectReference{
						UID:       string(5),
						Namespace: "bar",
						Name:      "foo",
					},
				},
				{
					MetricName: "requests_per_s_two",
					CustomTags: map[string]string{},
					Labels:     map[string]string{"foo": "bar"},
					Valid:      false,
					HPA: custommetrics.ObjectReference{
						UID:       string(7),
						Namespace: "baz",
						Name:      "foo",
					},
				},
			},
			[]custommetrics.ExternalMetricValue{
				{
					MetricName: "requests_per_s_two",
					CustomTags: map[string]string{},
					Labels:     map[string]string{"foo": "bar"},
					Valid:      false,
					HPA: custommetrics.ObjectReference{
						UID:       string(7),
						Name:      "foo",
						Namespace: "baz",
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
