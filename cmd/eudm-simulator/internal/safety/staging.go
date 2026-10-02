// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package safety resolves the simulator's isolated delivery configuration.
// Delivery must use these resolved destinations, never the host Agent config.
package safety

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
)

const Site = "datad0g.com"

// Destination is an Agent delivery route, including additional endpoints.
type Destination string

const (
	Metrics       Destination = "metrics"
	Metadata      Destination = "metadata"
	Processes     Destination = "processes"
	Connections   Destination = "connections"
	EventPlatform Destination = "event_platform"
	NDM           Destination = "ndm"
)

var destinations = []Destination{Metrics, Metadata, Processes, Connections, EventPlatform, NDM}

// Config is internal delivery configuration, separate from the live Agent
// configuration. The command reads Site from DD_SITE; capture uses Site without
// consulting the environment. Secrets are read at delivery time from DD_API_KEY
// and are never written to simulation artifacts.
type Config struct {
	Site      string
	Endpoints map[Destination][]string
}

// Resolve returns the complete destination set before any forwarder starts.
// getenv is injected so callers cannot silently inherit a production DD_SITE.
func (c Config) Resolve(getenv func(string) string) (map[Destination][]string, error) {
	if c.Site != Site {
		return nil, fmt.Errorf("DD_SITE must be %q; empty and production sites are forbidden", Site)
	}
	if site := getenv("DD_SITE"); site != "" && site != Site {
		return nil, errors.New("DD_SITE conflicts with required staging site")
	}
	for dest := range c.Endpoints {
		if !slices.Contains(destinations, dest) {
			return nil, fmt.Errorf("unsupported destination %q", dest)
		}
	}
	// Reject inherited Agent endpoint overrides so a later Agent constructor
	// cannot accidentally enable a second route.
	for _, key := range []string{"DD_DD_URL", "DD_URL", "DD_ADDITIONAL_ENDPOINTS", "DD_PROCESS_CONFIG_PROCESS_DD_URL", "DD_PROCESS_CONFIG_ADDITIONAL_ENDPOINTS", "DD_SOFTWARE_INVENTORY_FORWARDER_LOGS_DD_URL", "DD_SOFTWARE_INVENTORY_FORWARDER_ADDITIONAL_ENDPOINTS", "DD_NETWORK_DEVICES_METADATA_LOGS_DD_URL", "DD_NETWORK_DEVICES_METADATA_ADDITIONAL_ENDPOINTS", "DD_MULTI_REGION_FAILOVER_ENABLED"} {
		if getenv(key) != "" {
			return nil, fmt.Errorf("%s is not supported: unset it; simulator destinations are derived from DD_SITE", key)
		}
	}
	resolved := map[Destination][]string{
		Metrics: {"https://app." + Site}, Metadata: {"https://app." + Site},
		Processes: {"https://process." + Site}, Connections: {"https://process." + Site},
		EventPlatform: {"https://softinv-intake." + Site}, NDM: {"https://ndm-intake." + Site},
	}
	for _, dest := range destinations {
		if values, exists := c.Endpoints[dest]; exists {
			if len(values) == 0 {
				return nil, fmt.Errorf("destination %s cannot be disabled", dest)
			}
			resolved[dest] = slices.Clone(values)
		}
		for _, endpoint := range resolved[dest] {
			if err := ValidateEndpoint(endpoint); err != nil {
				return nil, fmt.Errorf("destination %s: %w", dest, err)
			}
		}
	}
	return resolved, nil
}

// ValidateEndpoint checks URLs without echoing potentially embedded credentials.
// HTTPS, a label-boundary domain match and no userinfo/query prevent ambiguous
// or externally redirected endpoint overrides. Forwarders must disable redirects.
func ValidateEndpoint(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return errors.New("invalid staging endpoint URL")
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if u.Scheme != "https" || u.User != nil || u.Opaque != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || (u.Port() != "" && u.Port() != "443") || (u.Path != "" && u.Path != "/") || (host != Site && !strings.HasSuffix(host, "."+Site)) {
		return fmt.Errorf("endpoint must be an HTTPS origin under %s", Site)
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return errors.New("invalid staging hostname")
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return errors.New("invalid staging hostname")
			}
		}
	}
	return nil
}
