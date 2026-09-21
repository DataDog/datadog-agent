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
// returns the parsed metric family matching metricName, or nil if it is not present in the
// response.
func FetchAPIServerMetricFamily(ctx context.Context, discoveryClient discovery.DiscoveryInterface, metricName string) (*prometheus.MetricFamily, error) {
	metricsData, err := discoveryClient.RESTClient().Get().AbsPath(apiServerMetricsPath).DoRaw(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query /metrics endpoint: %w", err)
	}

	families, err := prometheus.ParseMetrics(metricsData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse /metrics endpoint: %w", err)
	}

	for _, family := range families {
		if family.Name == metricName {
			return &family, nil
		}
	}

	return nil, nil
}
