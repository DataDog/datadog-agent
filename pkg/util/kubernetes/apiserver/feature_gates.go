// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package apiserver

import (
	"bufio"
	"context"
	"fmt"
	"regexp"
	"strings"
)

// FeatureGate represents a single Kubernetes feature gate
type FeatureGate struct {
	Name    string
	Stage   string // ALPHA, BETA, GA, DEPRECATED, or empty string for stable
	Enabled bool
}

// parseFeatureGatesFromMetrics parses the /metrics endpoint and extracts feature gate information
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

// GetClusterFeatureGates queries the /metrics endpoint and returns feature gates
func (c *APIClient) GetClusterFeatureGates(ctx context.Context) (map[string]FeatureGate, error) {
	metricsData, err := c.Cl.Discovery().RESTClient().Get().AbsPath("/metrics").DoRaw(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query /metrics endpoint: %v", err)
	}

	gates, err := parseFeatureGatesFromMetrics(metricsData)
	if err != nil {
		return nil, err
	}

	return gates, nil
}
