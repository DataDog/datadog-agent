// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package converterimpl provides the implementation of the otel-agent converter.
package converterimpl

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.opentelemetry.io/collector/confmap"
)

const opampExtensionName = "opamp"

// ensureOpampInstanceUID injects a stable instance UID into the opamp extension
// configuration. If the opamp extension is present in the config but has no
// instance_uid set, a UUID is read from (or generated and written to) the agent
// state directory so it persists across restarts.
//
// It also enriches the agent_description with DDOT identity attributes
// unconditionally, so that the OpAmp server always receives site and deployment
// type metadata even when the user supplies their own instance_uid.
//
// speky:DDOT#OTELCOL028
func (c *ddConverter) ensureOpampInstanceUID(conf *confmap.Conf) {
	stringMapConf := conf.ToStringMap()

	opampKey, opampCfg := findOpampExtension(stringMapConf)
	if opampKey == "" {
		return
	}

	if opampCfg == nil {
		opampCfg = make(map[string]any)
	}

	// Inject a persistent UID only when the user has not provided one.
	if uid, ok := opampCfg["instance_uid"]; !ok || uid == "" {
		if uid, err := c.loadOrCreateInstanceUID(); err == nil {
			opampCfg["instance_uid"] = uid
		}
	}

	// Always enrich agent_description regardless of who set the UID.
	c.enrichOpampAgentDescription(opampCfg)

	extensions := stringMapConf["extensions"].(map[string]any)
	extensions[opampKey] = opampCfg
	*conf = *confmap.NewFromStringMap(stringMapConf)
}

// enrichOpampAgentDescription injects DDOT deployment context into the opamp
// extension's agent_description.non_identifying_attributes when they are absent.
// This lets the OpAmp server know the site and deployment type without requiring
// the user to configure them manually.
//
// speky:DDOT#OTELCOL028
func (c *ddConverter) enrichOpampAgentDescription(opampCfg map[string]any) {
	if c.coreConfig == nil {
		return
	}

	// Retrieve or create the agent_description map.
	agentDesc, _ := opampCfg["agent_description"].(map[string]any)
	if agentDesc == nil {
		agentDesc = make(map[string]any)
	}

	// Retrieve or create the non_identifying_attributes map.
	nonIdent, _ := agentDesc["non_identifying_attributes"].(map[string]any)
	if nonIdent == nil {
		nonIdent = make(map[string]any)
	}

	if site := c.coreConfig.GetString("site"); site != "" {
		if _, already := nonIdent["datadoghq.com/site"]; !already {
			nonIdent["datadoghq.com/site"] = site
		}
	}

	deploymentType := "daemonset"
	if c.coreConfig.GetBool("otelcollector.gateway.mode") {
		deploymentType = "gateway"
	}
	if _, already := nonIdent["datadoghq.com/deployment_type"]; !already {
		nonIdent["datadoghq.com/deployment_type"] = deploymentType
	}

	agentDesc["non_identifying_attributes"] = nonIdent
	opampCfg["agent_description"] = agentDesc
}

// findOpampExtension returns the config key and config map for the opamp
// extension, searching for both "opamp" and "opamp/<name>" keys.
func findOpampExtension(stringMapConf map[string]any) (string, map[string]any) {
	extensions, ok := stringMapConf["extensions"].(map[string]any)
	if !ok {
		return "", nil
	}
	for key, val := range extensions {
		if componentName(key) == opampExtensionName {
			cfg, _ := val.(map[string]any)
			return key, cfg
		}
	}
	return "", nil
}

// loadOrCreateInstanceUID reads the persisted instance UID from disk. If the
// file does not exist yet, a new UUID v4 is generated and written.
//
// speky:DDOT#OTELCOL028
func (c *ddConverter) loadOrCreateInstanceUID() (string, error) {
	path := c.instanceUIDPath()

	if data, err := os.ReadFile(path); err == nil {
		if uid := strings.TrimSpace(string(data)); uid != "" {
			return uid, nil
		}
	}

	uid, err := generateUUID()
	if err != nil {
		return "", fmt.Errorf("generating opamp instance UID: %w", err)
	}

	// Best-effort persistence — a transient write failure is non-fatal; the
	// extension will simply get a new UID on the next restart.
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	_ = os.WriteFile(path, []byte(uid), 0o600)

	return uid, nil
}

// instanceUIDPath returns the path to the persisted instance UID file.
//
// speky:DDOT#OTELCOL028
func (c *ddConverter) instanceUIDPath() string {
	if c.coreConfig != nil {
		if runPath := c.coreConfig.GetString("run_path"); runPath != "" {
			return filepath.Join(runPath, "otel-instance-uid")
		}
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "datadog-agent", "otel-instance-uid")
}

// generateUUID produces a random UUID v4 string using crypto/rand.
func generateUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	// Set version (4) and variant bits per RFC 4122.
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
