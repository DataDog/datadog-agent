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
	// PrimaryEndpoint is the configured URL before normalization.
	PrimaryEndpoint string
	// PrimaryEndpointURL is the URL used by the forwarder.
	PrimaryEndpointURL string
}

// ReadMetricsEndpointRouting reads policies and validates their endpoint URLs.
func ReadMetricsEndpointRouting(c model.Reader) (MetricsEndpointRouting, error) {
	return readMetricsEndpointRouting(c, c.GetStringMapStringSlice("additional_endpoints"))
}

func readMetricsEndpointRouting(c model.Reader, additional map[string][]string) (MetricsEndpointRouting, error) {
	var routing MetricsEndpointRouting
	var err error
	if routing.MetricAllowlist, err = readMetricAllowlist(c, "metric_allowlist"); err != nil {
		return routing, err
	}
	if routing.MetricPrefixAllowlist, err = readMetricAllowlist(c, "metric_prefix_allowlist"); err != nil {
		return routing, err
	}
	hasPolicies := len(routing.MetricAllowlist) > 0 || len(routing.MetricPrefixAllowlist) > 0
	if hasPolicies && !c.GetBool("use_v2_api.series") {
		return routing, fmt.Errorf("%s requires use_v2_api.series to be enabled", MetricsEndpointRoutingKey)
	}
	configured, transport, err := resolveMetricsPrimaryEndpoint(c, additional, hasPolicies)
	if err != nil {
		return routing, err
	}
	routing.PrimaryEndpoint, routing.PrimaryEndpointURL = configured, transport
	for _, policies := range []map[string][]string{routing.MetricAllowlist, routing.MetricPrefixAllowlist} {
		for endpoint := range policies {
			if _, ok := additional[endpoint]; endpoint != configured && !ok {
				return routing, fmt.Errorf("%s: endpoint %q matches neither the primary endpoint %q nor a key in additional_endpoints", MetricsEndpointRoutingKey, endpoint, configured)
			}
		}
	}
	return routing, nil
}

// readMetricAllowlist validates endpoint-to-string-list mappings without coercing values.
func readMetricAllowlist(c model.Reader, field string) (map[string][]string, error) {
	key := MetricsEndpointRoutingKey + "." + field
	raw := c.Get(key)
	if raw == nil {
		return nil, nil
	}
	if policies, ok := raw.(map[string][]string); ok {
		for endpoint, names := range policies {
			if names == nil {
				return nil, fmt.Errorf("%s: endpoint %q must contain a list of strings, not null", key, endpoint)
			}
		}
		return policies, nil
	}
	entries, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s: expected a map of endpoint URLs to string lists, got %T", key, raw)
	}
	policies := make(map[string][]string, len(entries))
	for endpoint, value := range entries {
		var names []string
		switch list := value.(type) {
		case []string:
			names = list
		case []any:
			if list != nil {
				names = make([]string, len(list))
			}
			for i, item := range list {
				name, ok := item.(string)
				if !ok {
					return nil, fmt.Errorf("%s: endpoint %q entry %d must be a string, not %T", key, endpoint, i, item)
				}
				names[i] = name
			}
		}
		if names == nil {
			return nil, fmt.Errorf("%s: endpoint %q must contain a list of strings, not %T", key, endpoint, value)
		}
		policies[endpoint] = names
	}
	return policies, nil
}

func resolveMetricsPrimaryEndpoint(c model.Reader, additional map[string][]string, hasPolicies bool) (configured, transport string, err error) {
	configured = c.GetString("dd_url")
	if site := c.GetString("site"); !c.IsConfigured("dd_url") && site != "" {
		configured = InfraURLPrefix + site
	}
	transport = GetInfraEndpoint(c)
	if _, collision := additional[transport]; hasPolicies && transport != configured && collision {
		return "", "", fmt.Errorf("%s: primary endpoint %q normalizes to additional endpoint %q. To disambiguate their policies and credentials, set dd_url explicitly", MetricsEndpointRoutingKey, configured, transport)
	}
	return configured, transport, nil
}
