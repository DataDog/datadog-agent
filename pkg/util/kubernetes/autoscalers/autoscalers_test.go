// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

//go:build kubeapiserver

package autoscalers

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	autoscalingv2 "k8s.io/api/autoscaling/v2beta1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/DataDog/watermarkpodautoscaler/api/v1alpha1"

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
					ResourceVersion: "12",
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
					ResourceVersion: "14",
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
		"Resync": {
			&autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "17",
					Annotations:     map[string]string{},
				},
				Status: autoscalingv2.HorizontalPodAutoscalerStatus{
					CurrentReplicas: 12,
				},
			},
			&autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "17",
					Annotations:     map[string]string{},
				},
				Status: autoscalingv2.HorizontalPodAutoscalerStatus{
					CurrentReplicas: 12,
				},
			},
			true,
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
			val := AutoscalerMetricsUpdate(testCase.hpaNew.GetObjectMeta(), testCase.hpaOld.GetObjectMeta())
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
					Labels:     map[string]string{"dcos_version": "1.9.4"},
					Ref: custommetrics.ObjectReference{
						Type: "horizontal",
					},
					Timestamp: 0,
					Value:     0,
					Valid:     false,
				},
				{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"dcos_version": "2.1.9"},
					Ref: custommetrics.ObjectReference{
						Type: "horizontal",
					},
					Timestamp: 0,
					Value:     0,
					Valid:     false,
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
					Ref: custommetrics.ObjectReference{
						Type: "horizontal",
					},
					Labels:    nil,
					Timestamp: 0,
					Value:     0,
					Valid:     false,
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
		"skip invalid metric names": {
			&autoscalingv2.HorizontalPodAutoscaler{
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					Metrics: []autoscalingv2.MetricSpec{
						{
							Type: autoscalingv2.ExternalMetricSourceType,
							External: &autoscalingv2.ExternalMetricSource{
								MetricName: "valid_name",
							},
						},
						{
							Type: autoscalingv2.ExternalMetricSourceType,
							External: &autoscalingv2.ExternalMetricSource{
								MetricName: "valid_name.with_dots.and_underscores.AndUppercaseLetters",
							},
						},
						{
							Type: autoscalingv2.ExternalMetricSourceType,
							External: &autoscalingv2.ExternalMetricSource{
								MetricName: "0_invalid_name_must_start_with_letter",
							},
						},
						{
							Type: autoscalingv2.ExternalMetricSourceType,
							External: &autoscalingv2.ExternalMetricSource{
								MetricName: "spaces are invalid",
							},
						},
						{
							Type: autoscalingv2.ExternalMetricSourceType,
							External: &autoscalingv2.ExternalMetricSource{
								MetricName: "utf8_invalid_🤷",
							},
						},
						{
							Type: autoscalingv2.ExternalMetricSourceType,
							External: &autoscalingv2.ExternalMetricSource{
								MetricName: "over_200_characters_padding_padding_padding_padding_padding_padding_padding_padding_padding_padding_padding_padding_padding_padding_padding_padding_padding_padding_padding_padding_padding_padding_padding",
							},
						},
					},
				},
			},
			[]custommetrics.ExternalMetricValue{
				{
					MetricName: "valid_name",
					Ref: custommetrics.ObjectReference{
						Type: "horizontal",
					},
					Labels:    nil,
					Timestamp: 0,
					Value:     0,
					Valid:     false,
				},
				{
					MetricName: "valid_name.with_dots.and_underscores.AndUppercaseLetters",
					Ref: custommetrics.ObjectReference{
						Type: "horizontal",
					},
					Labels:    nil,
					Timestamp: 0,
					Value:     0,
					Valid:     false,
				},
			},
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
					Labels:     map[string]string{"dcos_version": "1.9.4"},
					Ref: custommetrics.ObjectReference{
						Type: "horizontal",
					},
					Timestamp: 0,
					Value:     0,
					Valid:     false,
				},
			},
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			got := InspectHPA(testCase.hpa)
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

func makeWPASpec(metricName string, labels map[string]string) v1alpha1.WatermarkPodAutoscalerSpec {
	return v1alpha1.WatermarkPodAutoscalerSpec{
		Metrics: []v1alpha1.MetricSpec{
			{
				Type: v1alpha1.ExternalMetricSourceType,
				External: &v1alpha1.ExternalMetricSource{
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
		informerHPAs  []metav1.Object
		informerWPAs  []*v1alpha1.WatermarkPodAutoscaler
		storedMetrics []custommetrics.ExternalMetricValue
		expected      []custommetrics.ExternalMetricValue
	}{
		"delete invalid metric": {

			[]metav1.Object{
				&autoscalingv2.HorizontalPodAutoscaler{
					ObjectMeta: metav1.ObjectMeta{
						UID:       types.UID(fmt.Sprint(5)),
						Namespace: "nsbar",
						Name:      "foo",
					},
					Spec: makeSpec("requests_per_s_one", map[string]string{"foo": "tagbar"}),
				},
				&autoscalingv2.HorizontalPodAutoscaler{
					ObjectMeta: metav1.ObjectMeta{
						UID:       types.UID(fmt.Sprint(7)),
						Namespace: "zanzi",
						Name:      "bar",
					},
					Spec: makeSpec("requests_per_s_three", map[string]string{"foo": "tu"}),
				},
			},
			[]*v1alpha1.WatermarkPodAutoscaler{
				{
					ObjectMeta: metav1.ObjectMeta{
						UID:       types.UID(fmt.Sprint(9)),
						Namespace: "nsbar",
						Name:      "foo",
					},
					Spec: makeWPASpec("requests_per_s_one", map[string]string{"foo": "tagbar"}),
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						UID:       types.UID(fmt.Sprint(11)),
						Namespace: "zanzi",
						Name:      "bar",
					},
					Spec: makeWPASpec("requests_per_s_three", map[string]string{"foo": "tu"}),
				},
			},
			[]custommetrics.ExternalMetricValue{
				{
					MetricName: "requests_per_s_one",
					Labels:     map[string]string{"foo": "tagbar"},
					Valid:      true,
					Ref: custommetrics.ObjectReference{
						Type:      "horizontal",
						UID:       fmt.Sprint(5),
						Name:      "foo",
						Namespace: "nsbar",
					},
				},
				{
					MetricName: "requests_per_s_two",
					Labels:     map[string]string{"foo": "dre"},
					Valid:      false,
					Ref: custommetrics.ObjectReference{
						Type:      "horizontal",
						UID:       fmt.Sprint(6),
						Name:      "foo",
						Namespace: "baz",
					},
				},
				{
					MetricName: "requests_per_s_three",
					Labels:     map[string]string{"foo": "tu"},
					Valid:      false,
					Ref: custommetrics.ObjectReference{
						Type:      "horizontal",
						UID:       fmt.Sprint(7),
						Name:      "bar",
						Namespace: "zanzi",
					},
				},
				{
					MetricName: "requests_per_s_three",
					Labels:     map[string]string{"foo": "tu"},
					Valid:      false,
					Ref: custommetrics.ObjectReference{
						Type:      "watermark",
						UID:       fmt.Sprint(9),
						Name:      "bar",
						Namespace: "zanzi",
					},
				},
			},
			[]custommetrics.ExternalMetricValue{
				{
					MetricName: "requests_per_s_two",
					Labels:     map[string]string{"foo": "dre"},
					Valid:      false,
					Ref: custommetrics.ObjectReference{
						Type:      "horizontal",
						UID:       fmt.Sprint(6),
						Name:      "foo",
						Namespace: "baz",
					},
				},
				{
					MetricName: "requests_per_s_three",
					Labels:     map[string]string{"foo": "tu"},
					Valid:      false,
					Ref: custommetrics.ObjectReference{
						Type:      "watermark",
						UID:       fmt.Sprint(9),
						Name:      "bar",
						Namespace: "zanzi",
					},
				},
			},
		},
		"metric name changed": {
			[]metav1.Object{
				&autoscalingv2.HorizontalPodAutoscaler{
					ObjectMeta: metav1.ObjectMeta{
						UID:       types.UID(fmt.Sprint(5)),
						Namespace: "bar",
						Name:      "foo",
					},
					Spec: makeSpec("requests_per_s_one", map[string]string{"foo": "bar"}),
				},
				&autoscalingv2.HorizontalPodAutoscaler{
					ObjectMeta: metav1.ObjectMeta{
						UID:       types.UID(fmt.Sprint(7)),
						Namespace: "baz",
						Name:      "foo",
					},
					Spec: makeSpec("requests_per_s_new", map[string]string{"foo": "bar"}),
				},
			},
			[]*v1alpha1.WatermarkPodAutoscaler{},
			[]custommetrics.ExternalMetricValue{
				{
					MetricName: "requests_per_s_one",
					Labels:     map[string]string{"foo": "bar"},
					Valid:      true,
					Ref: custommetrics.ObjectReference{
						Type:      "horizontal",
						UID:       fmt.Sprint(5),
						Namespace: "bar",
						Name:      "foo",
					},
				},
				{
					MetricName: "requests_per_s_old",
					Labels:     map[string]string{"foo": "bar"},
					Valid:      false,
					Ref: custommetrics.ObjectReference{
						Type:      "horizontal",
						UID:       fmt.Sprint(7),
						Namespace: "baz",
						Name:      "foo",
					},
				},
			},
			[]custommetrics.ExternalMetricValue{
				{
					MetricName: "requests_per_s_old",
					Labels:     map[string]string{"foo": "bar"},
					Valid:      false,
					Ref: custommetrics.ObjectReference{
						Type:      "horizontal",
						UID:       fmt.Sprint(7),
						Name:      "foo",
						Namespace: "baz",
					},
				},
			},
		},
		"legacy entry": {
			[]metav1.Object{},
			[]*v1alpha1.WatermarkPodAutoscaler{
				{
					ObjectMeta: metav1.ObjectMeta{
						UID:       types.UID(fmt.Sprint(7)),
						Namespace: "zanzi",
						Name:      "bar",
					},
					Spec: makeWPASpec("requests_per_s_three", map[string]string{"foo": "tu"}),
				},
			},
			[]custommetrics.ExternalMetricValue{
				{
					MetricName: "requests_per_s_three",
					Labels:     map[string]string{"foo": "tu"},
					Valid:      true,
					Ref: custommetrics.ObjectReference{
						UID:       fmt.Sprint(7),
						Name:      "bar",
						Namespace: "zanzi",
					},
				},
			},
			[]custommetrics.ExternalMetricValue{
				{
					MetricName: "requests_per_s_three",
					Labels:     map[string]string{"foo": "tu"},
					Valid:      true,
					Ref: custommetrics.ObjectReference{
						UID:       fmt.Sprint(7),
						Name:      "bar",
						Namespace: "zanzi",
					},
				},
			},
		},
		"metric labels changed": {
			[]metav1.Object{
				&autoscalingv2.HorizontalPodAutoscaler{
					ObjectMeta: metav1.ObjectMeta{
						UID:       types.UID(fmt.Sprint(5)),
						Namespace: "bar",
						Name:      "foo",
					},
					Spec: makeSpec("requests_per_s_one", map[string]string{"foo": "bar"}),
				},
				&autoscalingv2.HorizontalPodAutoscaler{
					ObjectMeta: metav1.ObjectMeta{
						UID:       types.UID(fmt.Sprint(7)),
						Namespace: "baz",
						Name:      "foo",
					},
					Spec: makeSpec("requests_per_s_two", map[string]string{"foo": "foobar"}),
				},
			},
			[]*v1alpha1.WatermarkPodAutoscaler{},
			[]custommetrics.ExternalMetricValue{
				{
					MetricName: "requests_per_s_one",
					Labels:     map[string]string{"foo": "bar"},
					Valid:      true,
					Ref: custommetrics.ObjectReference{
						Type:      "horizontal",
						UID:       fmt.Sprint(5),
						Namespace: "bar",
						Name:      "foo",
					},
				},
				{
					MetricName: "requests_per_s_two",
					Labels:     map[string]string{"foo": "bar"},
					Valid:      false,
					Ref: custommetrics.ObjectReference{
						Type:      "horizontal",
						UID:       fmt.Sprint(7),
						Namespace: "baz",
						Name:      "foo",
					},
				},
			},
			[]custommetrics.ExternalMetricValue{
				{
					MetricName: "requests_per_s_two",
					Labels:     map[string]string{"foo": "bar"},
					Valid:      false,
					Ref: custommetrics.ObjectReference{
						Type:      "horizontal",
						UID:       fmt.Sprint(7),
						Name:      "foo",
						Namespace: "baz",
					},
				},
			},
		},
		"upgrade from old template": {
			[]metav1.Object{
				&autoscalingv2.HorizontalPodAutoscaler{
					ObjectMeta: metav1.ObjectMeta{
						UID:       types.UID(fmt.Sprint(5)),
						Namespace: "bar",
						Name:      "foo",
					},
					Spec: makeSpec("requests_per_s_one", map[string]string{"foo": "bar"}),
				},
				&autoscalingv2.HorizontalPodAutoscaler{
					ObjectMeta: metav1.ObjectMeta{
						UID:       types.UID(fmt.Sprint(7)),
						Namespace: "baz",
						Name:      "foo",
					},
					Spec: makeSpec("requests_per_s_two", map[string]string{"foo": "foobar"}),
				},
			},
			[]*v1alpha1.WatermarkPodAutoscaler{},
			[]custommetrics.ExternalMetricValue{
				{
					MetricName: "requests_per_s_one",
					Labels:     map[string]string{"foo": "bar"},
					Valid:      true,
					Ref: custommetrics.ObjectReference{
						Type:      "horizontal",
						UID:       fmt.Sprint(5),
						Namespace: "bar",
						Name:      "foo",
					},
				},
				{
					MetricName: "requests_per_s_two",
					Labels:     map[string]string{"foo": "bar"},
					Valid:      false,
					Ref: custommetrics.ObjectReference{
						Type:      "horizontal",
						UID:       fmt.Sprint(7),
						Namespace: "baz",
						Name:      "foo",
					},
				},
			},
			[]custommetrics.ExternalMetricValue{
				{
					MetricName: "requests_per_s_two",
					Labels:     map[string]string{"foo": "bar"},
					Valid:      false,
					Ref: custommetrics.ObjectReference{
						Type:      "horizontal",
						UID:       fmt.Sprint(7),
						Name:      "foo",
						Namespace: "baz",
					},
				},
			},
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			got := DiffExternalMetrics(testCase.informerHPAs, testCase.informerWPAs, testCase.storedMetrics)
			assert.ElementsMatch(t, testCase.expected, got)
		})
	}
}
