// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package common

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/prometheus"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
	"k8s.io/client-go/discovery"
)

const (
	// API Server metrics path
	apiServerMetricsPath = "/metrics"
)

var (
	retrier retry.Retrier
)

// FeatureGate represents a single Kubernetes feature gate
type FeatureGate struct {
	Name    string
	Stage   string
	Enabled bool
}

// parseFeatureGatesFromMetrics parses the metrics endpoint, extracting feature gate information
// Expected format: kubernetes_feature_enabled{name="FeatureName",stage="STAGE"} 1
func parseFeatureGatesFromMetrics(metricsData []byte) (map[string]FeatureGate, error) {
	gates := make(map[string]FeatureGate)

	metrics, err := prometheus.ParseMetrics(metricsData)
	if err != nil {
		return nil, fmt.Errorf("error parsing feature gate metrics: %w", err)
	}

	for _, metric := range metrics {
		if metric.Name != "kubernetes_feature_enabled" {
			continue
		}

		for _, sample := range metric.Samples {
			name := sample.Metric["name"]
			stage := sample.Metric["stage"]
			// sample values are always either (0 or 1)
			enabled := sample.Value == 1
			gates[string(name)] = FeatureGate{
				Name:    string(name),
				Stage:   string(stage),
				Enabled: enabled,
			}
		}
	}
	return gates, nil
}

// ClusterFeatureGates returns a map of all feature gates enabled/disabled in the cluster.
// It retries using exponential backoff until timeout is reached.
// Results are cached for 1 hour.
func ClusterFeatureGates(ctx context.Context, discoveryClient discovery.DiscoveryInterface, timeout time.Duration) (map[string]FeatureGate, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var featureGates map[string]FeatureGate
	err := retrier.SetupRetrier(&retry.Config{
		Name: "featureGates",
		AttemptMethod: func() error {
			metricsData, err := discoveryClient.RESTClient().Get().AbsPath(apiServerMetricsPath).DoRaw(timeoutCtx)
			if err != nil {
				return fmt.Errorf("failed to query /metrics endpoint: %v", err)
			}

			featureGates, err = parseFeatureGatesFromMetrics(metricsData)
			if err != nil {
				return err
			}

			return nil
		},
		Strategy:          retry.Backoff,
		InitialRetryDelay: 1 * time.Second,
		MaxRetryDelay:     timeout,
	})
	if err != nil {
		return nil, fmt.Errorf("couldnt setup retrier: %w", err)
	}

	for {
		err = retrier.TriggerRetry()
		switch retrier.RetryStatus() {
		case retry.OK:
			return featureGates, nil
		case retry.PermaFail:
			return nil, err
		default:
			sleepFor := time.Until(retrier.NextRetry()) + time.Second
			log.Debugf("Waiting for APIServer, next retry: %v", sleepFor)
			select {
			case <-timeoutCtx.Done():
				return nil, errors.New("timeout reached while waiting for Kubernetes feature gates")
			case <-time.After(sleepFor):
			}
		}
	}
}
