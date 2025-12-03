// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"bufio"
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
	"k8s.io/client-go/discovery"
)

const (
	featureGatesCacheKey = "featureGates"

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

	// Regex to parse: kubernetes_feature_enabled{name="SomeFeature",stage="BETA"} 1
	pattern := regexp.MustCompile(`kubernetes_feature_enabled\{name="([^"]+)",stage="([^"]*)"\}\s+(\d+)`)

	scanner := bufio.NewScanner(strings.NewReader(string(metricsData)))
	for scanner.Scan() {
		line := scanner.Text()

		// Skip comments and non-matching lines
		if strings.HasPrefix(line, "#") || !strings.Contains(line, "kubernetes_feature_enabled") {
			continue
		}

		matches := pattern.FindStringSubmatch(line)
		if len(matches) == 4 {
			name := matches[1]
			stage := matches[2]
			enabled := matches[3] == "1"

			gates[name] = FeatureGate{
				Name:    name,
				Stage:   stage,
				Enabled: enabled,
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error scanning metrics: %v", err)
	}

	return gates, nil
}

// ClusterFeatureGates queries the /metrics endpoint and returns feature gates
func ClusterFeatureGates(ctx context.Context, discoveryClient discovery.DiscoveryInterface, _ time.Duration) (map[string]FeatureGate, error) {
	if featureGates, found := cache.Cache.Get(featureGatesCacheKey); found {
		return featureGates.(map[string]FeatureGate), nil
	}

	var featureGates map[string]FeatureGate
	err := retrier.SetupRetrier(&retry.Config{
		Name: "featureGates",
		AttemptMethod: func() error {
			metricsData, err := discoveryClient.RESTClient().Get().AbsPath(apiServerMetricsPath).DoRaw(ctx)
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
		MaxRetryDelay:     2 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("couldnt setup retrier: %w", err)
	}

	for {
		err = retrier.TriggerRetry()
		switch retrier.RetryStatus() {
		case retry.OK:
			cache.Cache.Set(featureGatesCacheKey, featureGates, time.Hour)
			return featureGates, nil
		case retry.PermaFail:
			return nil, err
		default:
			sleepFor := retrier.NextRetry().UTC().Sub(time.Now().UTC()) + time.Second
			log.Debugf("Waiting for APIServer, next retry: %v", sleepFor)
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("context finished while waiting for Kubernetes feature gates")
			case <-time.After(sleepFor):
			}
		}
	}
}
