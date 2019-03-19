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
	"github.com/stretchr/testify/require"
	datadog "gopkg.in/zorkian/go-datadog-api.v2"
	autoscalingv2 "k8s.io/api/autoscaling/v2beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/custommetrics"
	testutil "github.com/DataDog/datadog-agent/test/util"
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
		ts = (int(metav1.Now().Unix()) - int(maxAge.Seconds())) * 1000 // use ms
	}
	tsPtr := float64(ts)
	valPtr := float64(val)
	return datadog.DataPoint{&tsPtr, &valPtr}
}

func makePartialPoints(ts int) datadog.DataPoint {
	tsPtr := float64(ts)
	return datadog.DataPoint{&tsPtr, nil}
}

func makePtr(val string) *string {
	return &val
}

func TestProcessor_UpdateExternalMetrics(t *testing.T) {
	penTime := (int(time.Now().Unix()) - int(maxAge.Seconds()/2)) * 1000
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
					MetricName: metricName,
					CustomTags: map[string]string{},
					Labels:     map[string]string{"foo": "bar"},
					Valid:      false,
				},
			},
			[]datadog.Series{
				testutil.BuildSeriesWithDefaults(metricName, "foo:bar", []datadog.DataPoint{
					makePoints(1531492452000, 12),
					makePoints(penTime, 14), // Force the penultimate point to be considered fresh at all time(< externalMaxAge)
					makePoints(0, 27),
				}),
			},
			[]custommetrics.ExternalMetricValue{
				{
					MetricName: "requests_per_s",
					CustomTags: map[string]string{},
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
					CustomTags: map[string]string{},
					Labels:     map[string]string{"2foo": "bar"},
					Valid:      true,
				},
			},
			[]datadog.Series{
				testutil.BuildSeriesWithDefaults(metricName, "2foo:bar", []datadog.DataPoint{
					makePoints(1431492452000, 12),
					makePoints(1431492453000, 14), // Force the point to be considered outdated at all time(> externalMaxAge)
					makePoints(0, 1000),           // Force the point to be considered fresh at all time(< externalMaxAge)
				}),
			},
			[]custommetrics.ExternalMetricValue{
				{
					MetricName: "requests_per_s",
					CustomTags: map[string]string{},
					Labels:     map[string]string{"2foo": "bar"},
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

	// Test that Datadog not responding yields invaldation.
	emList := []custommetrics.ExternalMetricValue{
		{
			MetricName: metricName,
			CustomTags: map[string]string{},
			Labels:     map[string]string{"foo": "bar"},
			Valid:      true,
		},
		{
			MetricName: metricName,
			CustomTags: map[string]string{},
			Labels:     map[string]string{"bar": "baz"},
			Valid:      true,
		},
	}
	datadogClient := &fakeDatadogClient{
		queryMetricsFunc: func(int64, int64, string) ([]datadog.Series, error) {
			return nil, fmt.Errorf("API error 400 Bad Request: {\"error\": [\"Rate limit of 300 requests in 3600 seconds reqchec.\"]}")
		},
	}
	hpaCl := &Processor{datadogClient: datadogClient, externalMaxAge: maxAge}
	invList := hpaCl.UpdateExternalMetrics(emList)
	require.Len(t, invList, len(emList))
	for _, i := range invList {
		require.False(t, i.Valid)
	}

}

func TestProcessor_ProcessHPAs(t *testing.T) {
	penTime := (int(time.Now().Unix()) - int(maxAge.Seconds()/2)) * 1000
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
				testutil.BuildSeriesWithDefaults(metricName, "dcos_version:1.9.4", []datadog.DataPoint{
					makePoints(1531492452000, 12),
					makePoints(penTime, 14), // Force the penultimate point to be considered fresh at all time(< externalMaxAge)
					makePoints(0, 23),
				}),
			},
			[]custommetrics.ExternalMetricValue{
				{
					MetricName: "requests_per_s",
					CustomTags: map[string]string{},
					Labels:     map[string]string{"dcos_version": "1.9.4"},
					Value:      14,
					Valid:      true,
				},
			},
		},
		{
			"process valid hpa external metric with custom tags and aggregation function",
			autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"custom.datadog.agg":   "sum",
						"custom.datadog.tag.1": "1.9.4",
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
					},
				},
			},
			[]datadog.Series{
				testutil.BuildSeries(metricName, "sum", "dcos_version:1.9.4", []datadog.DataPoint{
					makePoints(1531492452000, 12),
					makePoints(penTime, 14), // Force the penultimate point to be considered fresh at all time(< externalMaxAge)
					makePoints(0, 23),
				}),
			},
			[]custommetrics.ExternalMetricValue{
				{
					MetricName:       "requests_per_s",
					CustomAggregator: "sum",
					CustomTags:       map[string]string{"custom.datadog.tag.1": "1.9.4"},
					Labels:           map[string]string{"dcos_version": "custom.datadog.tag.1"},
					Value:            14,
					Valid:            true,
				},
			},
		},
		{
			"process invalid hpa external metric",
			autoscalingv2.HorizontalPodAutoscaler{
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
					CustomTags: map[string]string{},
					Labels:     map[string]string{"dcos_version": "1.9.4"},
					Value:      0,
					Valid:      false,
				},
			},
		},
		{
			"process hpa external metrics",
			autoscalingv2.HorizontalPodAutoscaler{
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
				testutil.BuildSeriesWithDefaults(metricName, "dcos_version:1.9.4", []datadog.DataPoint{
					makePoints(1531492452000, 22),
					makePoints(penTime, 12),
					makePoints(0, 14),
				}),
				testutil.BuildSeriesWithDefaults(metricName, "dcos_version:2.1.9", []datadog.DataPoint{
					makePoints(1531492452000, 22),
					makePoints(penTime, 13), // Validate that there are 2 different entries for the metric metricName as there are 2 different scopes
					makePoints(0, 16),
				}),
				testutil.BuildSeriesWithDefaults(metricName, "dcos_version:3.1.1", []datadog.DataPoint{
					makePoints(1531492452000, 22),
					makePoints(1531492442000, 13), // old timestamp (over maxAge) - Keep the value, set to false
					makePoints(1531492432000, 10),
				}),
			},
			[]custommetrics.ExternalMetricValue{
				{
					MetricName: "requests_per_s",
					CustomTags: map[string]string{},
					Labels:     map[string]string{"dcos_version": "1.9.4"},
					Value:      12,
					Valid:      true,
				},
				{
					MetricName: "requests_per_s",
					CustomTags: map[string]string{},
					Labels:     map[string]string{"dcos_version": "2.1.9"},
					Value:      13,
					Valid:      true,
				},
				{
					MetricName: "requests_per_s",
					CustomTags: map[string]string{},
					Labels:     map[string]string{"dcos_version": "3.1.1"},
					Value:      13,
					Valid:      false,
				},
				{
					MetricName: "requests_per_s",
					CustomTags: map[string]string{},
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
					CustomTags: map[string]string{},
					Labels:     map[string]string{"foo": "bar"},
					Valid:      false,
				},
				{
					MetricName: "requests_per_s",
					CustomTags: map[string]string{},
					Labels:     map[string]string{"foo": "baz"},
					Valid:      false,
				},
				{
					MetricName: "requests_per_s",
					CustomTags: map[string]string{},
					Labels:     map[string]string{"foo": "foobar"},
					Valid:      false,
				},
			},
			[]datadog.Series{
				testutil.BuildSeriesWithDefaults(metricName, "foo:bar", []datadog.DataPoint{
					makePoints(1531492452000, 12),
					makePoints(0, 14),
				}),
			},
			1,
		},
		{
			"one metric to update",
			[]custommetrics.ExternalMetricValue{
				{
					MetricName: "requests_per_s",
					CustomTags: map[string]string{},
					Labels:     map[string]string{"foo": "bar"},
					Valid:      true,
				},
			},
			[]datadog.Series{
				testutil.BuildSeriesWithDefaults(metricName, "foo:bar", []datadog.DataPoint{
					makePoints(1431492452000, 12),
					makePoints(1431492453000, 14),
				}),
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

// Test that we consistently get the same key.
func TestBuildQuery(t *testing.T) {
	tests := []struct {
		desc          string
		metric        custommetrics.ExternalMetricValue
		expectedQuery string
	}{
		{
			"correct name and label",
			custommetrics.ExternalMetricValue{
				MetricName: "kubernetes.io",
				CustomTags: map[string]string{},
				Labels: map[string]string{
					"foo": "bar",
				},
			},
			testutil.BuildQueryWithDefaults("kubernetes.io{foo:bar}"),
		},
		{
			"correct name and labels",
			custommetrics.ExternalMetricValue{
				MetricName: "kubernetes.io",
				CustomTags: map[string]string{},
				Labels: map[string]string{
					"zfoo": "bar",
					"afoo": "bar",
					"ffoo": "bar",
				},
			},
			testutil.BuildQueryWithDefaults("kubernetes.io{afoo:bar,ffoo:bar,zfoo:bar}"),
		},
		{
			"correct name, no labels",
			custommetrics.ExternalMetricValue{
				MetricName: "kubernetes.io",
				CustomTags: map[string]string{},
				Labels:     make(map[string]string),
			},
			testutil.BuildQueryWithDefaults("kubernetes.io{*}"),
		},
		{
			"correct name, tag references and custom aggregrator",
			custommetrics.ExternalMetricValue{
				MetricName: "kubernetes.io",
				Labels: map[string]string{
					"kube_container_name": "custom.datadog.tag.1",
					"kube_deployment":     "custom.datadog.tag.2",
				},
				CustomAggregator: "sum",
				CustomTags: map[string]string{
					"custom.datadog.tag.1": "nginx",
					"custom.datadog.tag.2": "web",
				},
			},
			testutil.BuildQuery("sum", "kubernetes.io{kube_container_name:nginx,kube_deployment:web}"),
		},
	}
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			query := buildQuery(test.metric)
			require.Equal(t, test.expectedQuery, query)
		})
	}
}

func TestInvalidate(t *testing.T) {
	eml := []custommetrics.ExternalMetricValue{
		{
			MetricName: "foo",
			Valid:      false,
			Timestamp:  12,
		},
		{
			MetricName: "bar",
			Valid:      true,
			Timestamp:  1300,
		},
	}

	invalid := invalidate(eml)
	for _, e := range invalid {
		require.False(t, e.Valid)
		require.WithinDuration(t, time.Now(), time.Unix(e.Timestamp, 0), 5*time.Second)
	}
}
