// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build kubeapiserver

package hpa

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gopkg.in/zorkian/go-datadog-api.v2"
	autoscalingv2 "k8s.io/api/autoscaling/v2beta1"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/custommetrics"
)

type fakeDatadogClient struct {
	queryMetricsFunc func(from, to int64, query string) ([]datadog.Series, error)
}

func (d *fakeDatadogClient) QueryMetrics(from, to int64, query string) ([]datadog.Series, error) {
	if d.queryMetricsFunc != nil {
		return d.queryMetricsFunc(from, to, query)
	}
	return nil, nil
}

var maxAge = time.Duration(30 * time.Second)

func makePoints(ts, val int) datadog.DataPoint {
	if ts == 0 {
		ts = (int(metav1.Now().Unix()) - int(maxAge.Seconds()/2)) * 1000 // use ms
	}
	tsPtr := float64(ts)
	valPtr := float64(val)
	return datadog.DataPoint{&tsPtr, &valPtr}
}

func makePtr(val string) *string {
	return &val
}

func TestProcessor_UpdateExternalMetrics(t *testing.T) {
	metricName := "requests_per_s"
	tests := []struct {
		desc     string
		metrics  []custommetrics.ExternalMetricValue
		series   []datadog.Series
		expected []custommetrics.ExternalMetricValue
	}{
		{
			"update invalid metric",
			[]custommetrics.ExternalMetricValue{
				{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"foo": "bar"},
					Valid:      false,
				},
			},
			[]datadog.Series{
				{
					Metric: &metricName,
					Points: []datadog.DataPoint{
						makePoints(1531492452000, 12),
						makePoints(0, 14), // Force the point to be considered fresh at all time(< externalMaxAge)
					},
					Scope: makePtr("foo:bar"),
				},
			},
			[]custommetrics.ExternalMetricValue{
				{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"foo": "bar"},
					Value:      14,
					Valid:      true,
				},
			},
		},
		{
			"do not update valid sparse metric",
			[]custommetrics.ExternalMetricValue{
				{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"foo": "bar"},
					Valid:      true,
				},
			},
			[]datadog.Series{
				{
					Metric: &metricName,
					Points: []datadog.DataPoint{
						makePoints(1431492452000, 12),
						makePoints(1431492453000, 14), // Force the point to be considered outdated at all time(> externalMaxAge)
					},
					Scope: makePtr("foo:bar"),
				},
			},
			[]custommetrics.ExternalMetricValue{
				{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"foo": "bar"},
					Value:      14,
					Valid:      false,
				},
			},
		},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("#%d %s", i, tt.desc), func(t *testing.T) {
			datadogClient := &fakeDatadogClient{
				queryMetricsFunc: func(int64, int64, string) ([]datadog.Series, error) {
					return tt.series, nil
				},
			}
			hpaCl := &Processor{datadogClient: datadogClient, externalMaxAge: maxAge}

			externalMetrics := hpaCl.UpdateExternalMetrics(tt.metrics)
			// Timestamps are always set to time.Now() so we cannot assert the value
			// in a unit test.
			strippedTs := make([]custommetrics.ExternalMetricValue, 0)
			for _, m := range externalMetrics {
				m.Timestamp = 0
				strippedTs = append(strippedTs, m)
			}

			assert.ElementsMatch(t, tt.expected, strippedTs)
		})
	}
}

func TestProcessor_ProcessHPAs(t *testing.T) {
	metricName := "requests_per_s"
	tests := []struct {
		desc     string
		metrics  autoscalingv2.HorizontalPodAutoscaler
		series   []datadog.Series
		expected []custommetrics.ExternalMetricValue
	}{
		{
			"process valid hpa external metric",
			autoscalingv2.HorizontalPodAutoscaler{
				Status: autoscalingv2.HorizontalPodAutoscalerStatus{
					Conditions: []autoscalingv2.HorizontalPodAutoscalerCondition{
						{
							Type:   autoscalingv2.AbleToScale,
							Status: v1.ConditionTrue,
						},
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
										"dcos_version": "1.9.4",
									},
								},
							},
						},
					},
				},
			},
			[]datadog.Series{
				{
					Metric: &metricName,
					Points: []datadog.DataPoint{
						makePoints(1531492452000, 12),
						makePoints(0, 14),
					},
					Scope: makePtr("dcos_version:1.9.4"),
				},
			},
			[]custommetrics.ExternalMetricValue{
				{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"dcos_version": "1.9.4"},
					Value:      14,
					Valid:      true,
				},
			},
		},
		{
			"process invalid hpa external metric",
			autoscalingv2.HorizontalPodAutoscaler{
				Status: autoscalingv2.HorizontalPodAutoscalerStatus{
					Conditions: []autoscalingv2.HorizontalPodAutoscalerCondition{
						{
							Type:   autoscalingv2.AbleToScale,
							Status: v1.ConditionTrue,
						},
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
										"dcos_version": "1.9.4",
									},
								},
							},
						},
					},
				},
			},
			[]datadog.Series{
				{
					Metric: &metricName,
					Points: nil,
				},
			},
			[]custommetrics.ExternalMetricValue{
				{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"dcos_version": "1.9.4"},
					Value:      0,
					Valid:      false,
				},
			},
		},
		{
			"process hpa external metrics",
			autoscalingv2.HorizontalPodAutoscaler{
				Status: autoscalingv2.HorizontalPodAutoscalerStatus{
					Conditions: []autoscalingv2.HorizontalPodAutoscalerCondition{
						{
							Type:   autoscalingv2.AbleToScale,
							Status: v1.ConditionTrue,
						},
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
						{
							Type: autoscalingv2.ExternalMetricSourceType,
							External: &autoscalingv2.ExternalMetricSource{
								MetricName: metricName,
								MetricSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										"dcos_version": "3.1.1",
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
										"dcos_version": "4.1.1",
									},
								},
							},
						},
					},
				},
			},
			[]datadog.Series{
				{
					Metric: &metricName,
					Points: []datadog.DataPoint{
						makePoints(1531492452000, 22),
						makePoints(0, 12),
					},
					Scope: makePtr("dcos_version:1.9.4"),
				}, {
					Metric: &metricName,
					Points: []datadog.DataPoint{
						makePoints(1531492452000, 22),
						makePoints(0, 13), // Validate that there are 2 different entries for the metric metricName as there are 2 different scopes
					},
					Scope: makePtr("dcos_version:2.1.9"),
				},
				{
					Metric: &metricName,
					Points: []datadog.DataPoint{
						makePoints(1531492452000, 22),
						makePoints(1531492432000, 13), // old timestamp (over maxAge) - Keep the value, set to false
					},
					Scope: makePtr("dcos_version:3.1.1"),
				},
			},
			[]custommetrics.ExternalMetricValue{
				{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"dcos_version": "1.9.4"},
					Value:      12,
					Valid:      true,
				},
				{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"dcos_version": "2.1.9"},
					Value:      13,
					Valid:      true,
				},
				{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"dcos_version": "3.1.1"},
					Value:      13,
					Valid:      false,
				},
				{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"dcos_version": "4.1.1"},
					Value:      0, // If Datadog does not even return the metric, store it as invalid with Value = 0
					Valid:      false,
				},
			},
		},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("#%d %s", i, tt.desc), func(t *testing.T) {
			datadogClient := &fakeDatadogClient{
				queryMetricsFunc: func(int64, int64, string) ([]datadog.Series, error) {

					return tt.series, nil
				},
			}
			hpaCl := &Processor{datadogClient: datadogClient, externalMaxAge: maxAge}

			externalMetrics := hpaCl.ProcessHPAs(&tt.metrics)

			// Timestamps are always set to time.Now() so we cannot assert the value
			// in a unit test.
			strippedTs := make([]custommetrics.ExternalMetricValue, 0)
			for _, m := range externalMetrics {
				m.Timestamp = 0
				strippedTs = append(strippedTs, m)
			}
			assert.ElementsMatch(t, tt.expected, strippedTs)
		})
	}
}

func TestProcessor_Batching(t *testing.T) {
	metricName := "requests_per_s"
	tests := []struct {
		desc             string
		metrics          []custommetrics.ExternalMetricValue
		series           []datadog.Series
		expectedNumCalls int
	}{
		{
			"multiple metrics to update",
			[]custommetrics.ExternalMetricValue{
				{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"foo": "bar"},
					Valid:      false,
				},
				{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"foo": "baz"},
					Valid:      false,
				},
				{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"foo": "foobar"},
					Valid:      false,
				},
			},
			[]datadog.Series{
				{
					Metric: &metricName,
					Points: []datadog.DataPoint{
						makePoints(1531492452000, 12),
						makePoints(0, 14),
					},
					Scope: makePtr("foo:bar"),
				},
			},
			1,
		},
		{
			"one metric to update",
			[]custommetrics.ExternalMetricValue{
				{
					MetricName: "requests_per_s",
					Labels:     map[string]string{"foo": "bar"},
					Valid:      true,
				},
			},
			[]datadog.Series{
				{
					Metric: &metricName,
					Points: []datadog.DataPoint{
						makePoints(1431492452000, 12),
						makePoints(1431492453000, 14),
					},
					Scope: makePtr("foo:bar"),
				},
			},
			1,
		},
	}

	for i, tt := range tests {
		var batchCallsNum int
		t.Run(fmt.Sprintf("#%d %s", i, tt.desc), func(t *testing.T) {
			datadogClient := &fakeDatadogClient{
				queryMetricsFunc: func(int64, int64, string) ([]datadog.Series, error) {
					batchCallsNum++
					return tt.series, nil
				},
			}
			hpaCl := &Processor{datadogClient: datadogClient, externalMaxAge: maxAge}
			hpaCl.UpdateExternalMetrics(tt.metrics)
			assert.Equal(t, tt.expectedNumCalls, batchCallsNum)
		})
	}
}
