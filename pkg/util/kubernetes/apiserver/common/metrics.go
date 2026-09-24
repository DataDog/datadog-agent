// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package common

import (
	"context"
	"fmt"

	"k8s.io/client-go/discovery"

	"github.com/DataDog/datadog-agent/pkg/util/prometheus"
)

// FetchAPIServerMetricFamily queries the API server's /metrics endpoint once (no retry) and
// returns the first parsed metric family present in the response, in the order specified by
// metricNames. It returns nil if none of the requested families are present.
func FetchAPIServerMetricFamily(ctx context.Context, discoveryClient discovery.DiscoveryInterface, metricNames ...string) (*prometheus.MetricFamily, error) {
	metricsData, err := discoveryClient.RESTClient().Get().AbsPath(apiServerMetricsPath).DoRaw(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query /metrics endpoint: %w", err)
	}

	families, err := prometheus.ParseMetrics(metricsData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse /metrics endpoint: %w", err)
	}

	return findMetricFamily(families, metricNames...), nil
}

func findMetricFamily(families []prometheus.MetricFamily, metricNames ...string) *prometheus.MetricFamily {
	for _, metricName := range metricNames {
		for i := range families {
			if families[i].Name == metricName {
				return &families[i]
			}
		}
	}

	return nil
}
