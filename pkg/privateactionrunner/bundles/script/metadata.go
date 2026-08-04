// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_script

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ScriptMetadata is stored as scripts/metadata.json inside a catalog OCI package.
// It carries everything the PAR needs to execute the script without any user
// configuration: the command, its parameter schema, which host env vars are allowed
// in, and any env vars that should always be set when the script runs.
type ScriptMetadata struct {
	// Command is the argv to execute. The first element is a script filename
	// relative to the scripts/ dir in the package; the resolver expands it to
	// an absolute path before execution.
	Command []string `json:"command"`

	// ParameterSchema is a JSON Schema describing the parameters the caller must
	// supply. Validated before execution.
	ParameterSchema map[string]interface{} `json:"parameterSchema,omitempty"`

	// ParameterEnvMapping maps parameter names to environment variable names.
	// When set, the resolver injects each parameter value as the corresponding
	// env var so shell scripts can read inputs via $VAR instead of command args.
	// Example: {"releaseName": "RELEASE_NAME", "chart": "CHART"}
	ParameterEnvMapping map[string]string `json:"parameterEnvMapping,omitempty"`

	// AllowedEnvVars lists env var names whose values are forwarded from the
	// host environment (e.g. "KUBECONFIG", "HELM_NAMESPACE").
	AllowedEnvVars []string `json:"allowedEnvVars,omitempty"`

	// EnvVars are env vars always set when the script runs, regardless of host
	// environment or parameters. Useful for non-interactive tool flags, etc.
	EnvVars map[string]string `json:"envVars,omitempty"`
}

func readScriptMetadata(installDir string) (*ScriptMetadata, error) {
	metaPath := filepath.Join(installDir, "scripts", "metadata.json")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, fmt.Errorf("could not read script metadata from %s: %w", metaPath, err)
	}
	var meta ScriptMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("could not parse script metadata: %w", err)
	}
	if len(meta.Command) == 0 {
		return nil, fmt.Errorf("script metadata has no command")
	}
	return &meta, nil
}
