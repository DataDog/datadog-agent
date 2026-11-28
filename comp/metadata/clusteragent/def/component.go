// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package clusteragent is the metadata provider for datadog-cluster-agent process
package clusteragent

import "net/http"

// team: container-platform

// Component is the component type.
type Component interface {
	// WritePayloadAsJSON writes the payload as JSON to the response writer. It is used by cluster-agent metadata endpoint.
	WritePayloadAsJSON(w http.ResponseWriter, _ *http.Request)
	// SetRCClientID records the ID of this cluster-agent's Remote Config client, reported as
	// `rc_client_id` in the metadata payload so the backend can target this replica (e.g. for
	// flares). The RC client is built after the component, so it is set once it exists.
	SetRCClientID(id string)
}
