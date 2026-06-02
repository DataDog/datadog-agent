// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubelet

package kubelet

import "fmt"

// ErrForbidden is returned when a kubelet API call receives a 403 Forbidden response.
// It carries the full endpoint URL, the RBAC resource, and the HTTP verb.
type ErrForbidden struct {
	Endpoint string
	Resource string
	Verb     string
}

func (e *ErrForbidden) Error() string {
	return fmt.Sprintf("kubelet API returned 403 Forbidden for %s %s (%s)", e.Verb, e.Resource, e.Endpoint)
}

// KubeletPathToResource maps a kubelet API path to the corresponding Kubernetes RBAC sub-resource.
func KubeletPathToResource(path string) string {
	switch {
	case len(path) >= 14 && path[:14] == "/stats/summary":
		return "nodes/stats"
	case len(path) >= 15 && path[:15] == "/metrics/cadvis":
		return "nodes/metrics"
	case len(path) >= 8 && path[:8] == "/metrics":
		return "nodes/metrics"
	case len(path) >= 7 && path[:7] == "/healthz":
		return "nodes/proxy"
	case len(path) >= 5 && path[:5] == "/spec":
		return "nodes/spec"
	default:
		return "nodes/proxy"
	}
}
