// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package bundle

import (
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/schema"
)

var wireHeaders = []string{
	"Content-Type", "Content-Encoding", "DD-Agent-Payload", "X-Dd-Hostname", "X-Dd-Processagentversion", "X-Dd-Request-Id",
	"X-DD-Agent-Timestamp", "X-DD-Agent-Start-Time", "X-DD-Payload-Source", "X-DD-Processes-Enabled", "X-DD-Service-Discovery-Enabled",
}

func validateWire(ref SampleRef, wire WireReference) error {
	if len(wire.Body) == 0 || !validWirePath(ref, wire.Path) {
		return errors.New("invalid sanitized Agent wire reference")
	}
	seen := map[string]bool{}
	for key, values := range wire.Headers {
		canonical := http.CanonicalHeaderKey(key)
		if seen[canonical] || !slices.ContainsFunc(wireHeaders, func(allowed string) bool { return strings.EqualFold(key, allowed) }) ||
			len(values) != 1 || strings.ContainsAny(values[0], "\r\n") || len(values[0]) > 4096 {
			return errors.New("unsafe or invalid sanitized wire headers")
		}
		seen[canonical] = true
	}
	return nil
}

func validWirePath(ref SampleRef, path string) bool {
	switch ref.Stream {
	case schema.Metrics, schema.HostMetadata, schema.AgentInventory, schema.HostInventory, schema.HostSystemInfo:
		return slices.ContainsFunc(ref.Routes, func(route RoutingEvidence) bool { return route.Endpoint == path })
	case schema.Processes:
		return path == "/api/v1/collector"
	case schema.Connections:
		return path == "/api/v1/connections"
	case schema.Software:
		return path == "/api/v2/softinv" || path == "/api/v2/logs"
	default:
		return false
	}
}
