// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package com_datadoghq_remoteaction_agent provides Private Action Runner actions that
// operate on the local datadog-agent through its authenticated IPC HTTP API.
//
// The actions in this bundle run inside the Private Action Runner process,
// which is co-located with the core agent, and reach the agent the same way the
// agent's own CLI subcommands do: over the local HTTPS command API, reusing the
// agent's IPC auth token and certificate. They deliberately ignore any Action
// Platform connection credential — the target is always the local agent.
//
// Because targeting is local-only, this bundle must not be registered in the
// Cluster Agent's bundle registry (registry_kubeapiserver.go): there, "local"
// resolves to the Cluster Agent itself, not a specific node's core agent.
package com_datadoghq_remoteaction_agent

import (
	"encoding/json"
	"fmt"
	"net"
	"strconv"

	"go.yaml.in/yaml/v3"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

// agentBaseURL returns the base URL (scheme + host + port) of the local agent's
// IPC command API, e.g. "https://127.0.0.1:5001".
func agentBaseURL() (string, error) {
	ipcAddress, err := pkgconfigsetup.GetIPCAddress(pkgconfigsetup.Datadog())
	if err != nil {
		return "", fmt.Errorf("failed to get agent IPC address: %w", err)
	}
	port := pkgconfigsetup.Datadog().GetInt("cmd_port")
	return "https://" + net.JoinHostPort(ipcAddress, strconv.Itoa(port)), nil
}

// decodeAgentObject decodes an agent JSON response into a map. Action outputs
// are surfaced to callers as a protobuf struct, which must be a JSON object at
// the top level, so responses that decode to a non-object (or that are not JSON
// at all) are wrapped so the action always returns an object.
func decodeAgentObject(b []byte) map[string]interface{} {
	var v interface{}
	if err := json.Unmarshal(b, &v); err != nil {
		return map[string]interface{}{"raw": string(b)}
	}
	if obj, ok := v.(map[string]interface{}); ok {
		return obj
	}
	return map[string]interface{}{"value": v}
}

// decodeAgentYAML decodes a YAML agent response (such as the full agent
// configuration returned by /agent/config) into a JSON-serializable object. It
// falls back to returning the raw text under a "raw" key when the body is not a
// YAML mapping. yaml/v3 decodes mappings with string keys, so the result
// marshals cleanly to JSON.
func decodeAgentYAML(b []byte) map[string]interface{} {
	var v map[string]interface{}
	if err := yaml.Unmarshal(b, &v); err != nil || v == nil {
		return map[string]interface{}{"raw": string(b)}
	}
	return v
}
