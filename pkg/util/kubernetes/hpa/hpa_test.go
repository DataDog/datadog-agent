// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build kubeapiserver

package hpa

import (
	"fmt"
	"testing"
	autoscalingv2 "k8s.io/api/autoscaling/v2beta1"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/custommetrics"
	"github.com/stretchr/testify/assert"
	"gopkg.in/zorkian/go-datadog-api.v2"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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

func TestHPAProcessor_UpdateExternalMetrics(t *testing.T) {
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
						{1531492452, 12},
						{1531492486, 14},
					},
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
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("#%d %s", i, tt.desc), func(t *testing.T) {
			datadogClient := &fakeDatadogClient{
				queryMetricsFunc: func(int64, int64, string) ([]datadog.Series, error) {
					return tt.series, nil
				},
			}
			hpaCl := &HPAProcessor{datadogClient: datadogClient}

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


func TestHPAProcessor_ComputeDeleteExternalMetrics(t *testing.T) {
	tests := []struct {
		desc     string
		list  autoscalingv2.HorizontalPodAutoscalerList
		emList   []custommetrics.ExternalMetricValue
		expected []custommetrics.ExternalMetricValue
	}{
		{
			"Delete invalid metric",
			autoscalingv2.HorizontalPodAutoscalerList{
				Items: []autoscalingv2.HorizontalPodAutoscaler{
					{
						ObjectMeta: v1.ObjectMeta{
							UID: types.UID(5),
						},
					},
					{
						ObjectMeta: v1.ObjectMeta{
							UID: types.UID(7),
						},
					},

				},
			},
			[]custommetrics.ExternalMetricValue{
				{
					MetricName: "requests_per_s_one",
					Labels:     map[string]string{"foo": "bar"},
					Valid:      true,
					HPA:	custommetrics.ObjectReference{
						UID: string(5),
					},
				},
				{
					MetricName: "requests_per_s_two",
					Labels:     map[string]string{"foo": "bar"},
					Valid:      false,
					HPA:	custommetrics.ObjectReference{
						UID: string(6),
					},
				},
				{
					MetricName: "requests_per_s_three",
					Labels:     map[string]string{"foo": "bar"},
					Valid:      false,
					HPA:	custommetrics.ObjectReference{
						UID: string(7),
					},
				},
			},
			[]custommetrics.ExternalMetricValue{
				{
					MetricName: "requests_per_s_two",
					Labels:     map[string]string{"foo": "bar"},
					Valid:      false,
					HPA:	custommetrics.ObjectReference{
						UID: string(6),
					},
				},
			},
		},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("#%d %s", i, tt.desc), func(t *testing.T) {
			datadogClient := &fakeDatadogClient{
				queryMetricsFunc: func(int64, int64, string) ([]datadog.Series, error) {
					return nil, nil
				},
			}
			hpaCl := &HPAProcessor{datadogClient: datadogClient}

			externalMetrics := hpaCl.ComputeDeleteExternalMetrics(&tt.list, tt.emList)

			assert.ElementsMatch(t, tt.expected, externalMetrics)
		})
	}
}

