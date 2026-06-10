// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package ndmconnectivitycheck is a light component that performs an on-demand,
// credential-free connectivity (ICMP) check against device IPs and reports the
// per-device reachability back to the backend. It is the creds-free counterpart
// to the snmpscan device scan, intended for the NDM onboarding "validation" step.
package ndmconnectivitycheck

// team: network-device-monitoring-core

// Component is the component type.
type Component interface {
	// CheckConnectivity pings each IP in the request and sends the per-device
	// reachability result to the network-devices-metadata event platform.
	CheckConnectivity(req Request)
}

// Request describes an on-demand connectivity check.
type Request struct {
	// DeviceIPs is the set of device IPs to ping. IPv4 only for now.
	DeviceIPs []string
	// Namespace is the NDM namespace the results are reported under. When empty,
	// the Agent's configured `network_devices.namespace` (or "default") is used.
	Namespace string
}
