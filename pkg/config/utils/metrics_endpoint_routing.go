// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/config/model"
)

// MetricsEndpointRoutingKey is the configuration namespace for per-endpoint metric policies.
const MetricsEndpointRoutingKey = "experimental.metrics_endpoint_routing"

// MetricsEndpointRouting defines metric allowlists for primary and additional endpoints.
// Endpoints without a policy receive all metrics. Empty policies send none.
type MetricsEndpointRouting struct {
	MetricAllowlist       map[string][]string
	MetricPrefixAllowlist map[string][]string
	// PrimaryEndpointURL is the URL used by the forwarder.
	PrimaryEndpointURL string
}

// ReadMetricsEndpointRouting reads policies and validates their endpoint URLs.
func ReadMetricsEndpointRouting(c model.Reader) (MetricsEndpointRouting, error) {
	return readMetricsEndpointRouting(c, c.GetStringMapStringSlice("additional_endpoints"))
}

func readMetricsEndpointRouting(c model.Reader, additional map[string][]string) (MetricsEndpointRouting, error) {
	var routing MetricsEndpointRouting
	routing.MetricAllowlist = readMetricAllowlist(c, "metric_allowlist")
	routing.MetricPrefixAllowlist = readMetricAllowlist(c, "metric_prefix_allowlist")
	hasPolicies := len(routing.MetricAllowlist) > 0 || len(routing.MetricPrefixAllowlist) > 0
	if hasPolicies && !c.GetBool("use_v2_api.series") {
		return routing, fmt.Errorf("%s requires use_v2_api.series to be enabled", MetricsEndpointRoutingKey)
	}
	routing.PrimaryEndpointURL = GetInfraEndpoint(c)
	for _, policies := range []map[string][]string{routing.MetricAllowlist, routing.MetricPrefixAllowlist} {
		for endpoint := range policies {
			if _, ok := additional[endpoint]; endpoint != routing.PrimaryEndpointURL && !ok {
				return routing, fmt.Errorf("%s: endpoint %q matches neither the primary endpoint %q nor a key in additional_endpoints", MetricsEndpointRoutingKey, endpoint, routing.PrimaryEndpointURL)
			}
		}
	}
	return routing, nil
}

// readMetricAllowlist validates endpoint-to-string-list mappings without coercing values.
func readMetricAllowlist(c model.Reader, field string) map[string][]string {
	return c.GetStringMapStringSlice(MetricsEndpointRoutingKey + "." + field)
}
