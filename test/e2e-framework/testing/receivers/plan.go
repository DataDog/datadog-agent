// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package receivers describes outbound Agent destinations. Plans are pure data:
// no provisioning, credential lookup, Pulumi, or CLI dependencies belong here.
package receivers

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// DummyAPIKey is deliberately public, and is never a Datadog credential.
const DummyAPIKey = "00000000000000000000000000000000"

// Plan describes one managed Agent's destination, not other workload senders or
// fixture forwarding. It contains references, never resolved secrets.
type Plan struct {
	Type         string   `json:"type"`
	Endpoint     string   `json:"endpoint,omitempty"`
	QueryURL     string   `json:"queryURL,omitempty"`
	Site         string   `json:"site,omitempty"`
	APIKeyRef    string   `json:"apiKeyRef,omitempty"`
	RemoteConfig string   `json:"remoteConfig"`
	Warnings     []string `json:"warnings,omitempty"`
}

// EndpointURL validates an intake origin, not an arbitrary proxy path. Reject
// credentials and query strings so diagnostics and persisted plans are safe.
func EndpointURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || (u.Path != "" && u.Path != "/") || u.RawPath != "" {
		return "", fmt.Errorf("receiver endpoint must be an HTTP(S) origin without credentials, path, query or fragment")
	}
	if strings.ContainsAny(u.Host, "\\ \t\n\r") {
		return "", fmt.Errorf("invalid receiver host")
	}
	if p := u.Port(); p != "" {
		n, e := strconv.Atoi(p)
		if e != nil || n < 1 || n > 65535 {
			return "", fmt.Errorf("invalid receiver port")
		}
	}
	if strings.HasSuffix(u.Host, ":") {
		return "", fmt.Errorf("invalid receiver port")
	}
	u.Path = ""
	return u.String(), nil
}

// Capture uses an explicitly exported Agent-network endpoint. QueryURL is only
// for inspection; a reachable query endpoint is not proof of producer delivery.
func Capture(kind, endpoint, queryURL string) (Plan, error) {
	endpoint, err := EndpointURL(endpoint)
	if err != nil {
		return Plan{}, err
	}
	return Plan{Type: kind, Endpoint: endpoint, QueryURL: queryURL, RemoteConfig: "disabled", Warnings: []string{"Main Agent routing only; arbitrary checks, sidecars, independent senders and fixture forwarding are outside this policy.", "Delivery is unverified; readiness is not ingestion evidence.", "EVP and OpenLineage proxies are disabled for custom intakes until their distinct protocols are supported.", "If APM is running, manually or instruction-triggered tracer flare uses the native site, not this receiver. Diagnostic uploads are excluded; this is not a no-native-egress/isolation mode."}}, nil
}

// Datadog requires a deliberate site and bounded runner reference.
func Datadog(site, ref string) (Plan, error) {
	allowed := map[string]bool{"datadoghq.com": true, "datadoghq.eu": true, "us3.datadoghq.com": true, "us5.datadoghq.com": true, "ap1.datadoghq.com": true, "ap2.datadoghq.com": true, "ddog-gov.com": true}
	if !allowed[site] {
		return Plan{}, fmt.Errorf("datadog.site: unsupported site")
	}
	if ref != "runner/api_key" {
		return Plan{}, fmt.Errorf("datadog.api-key-ref: only runner/api_key is supported")
	}
	return Plan{Type: "datadog", Site: site, APIKeyRef: ref, RemoteConfig: "native", Warnings: []string{"Delivery is unverified; no Datadog backend query is performed."}}, nil
}

// Validate also protects public installer callers constructing plans directly.
func (p Plan) Validate() error {
	if p.Endpoint == "" {
		_, err := Datadog(p.Site, p.APIKeyRef)
		if err != nil {
			return err
		}
		if p.RemoteConfig != "native" {
			return fmt.Errorf("native routing requires native RC policy")
		}
	} else {
		if _, err := EndpointURL(p.Endpoint); err != nil {
			return err
		}
		if p.Site != "" || p.APIKeyRef != "" {
			return fmt.Errorf("custom intake cannot receive native credentials or a site")
		}
		if p.RemoteConfig != "disabled" {
			return fmt.Errorf("receiver Remote Config is not implemented; select disabled explicitly")
		}
	}
	return nil
}

// AgentURL rejects operator loopback when the producer is in another network.
func AgentURL(raw string, separateNetwork bool) (string, error) {
	endpoint, err := EndpointURL(raw)
	if err != nil {
		return "", err
	}
	u, _ := url.Parse(endpoint)
	ip := net.ParseIP(u.Hostname())
	if separateNetwork && (strings.EqualFold(u.Hostname(), "localhost") || (ip != nil && (ip.IsLoopback() || ip.IsUnspecified()))) {
		return "", fmt.Errorf("receiver Agent URL is loopback/unspecified in a separate producer network")
	}
	return endpoint, nil
}
