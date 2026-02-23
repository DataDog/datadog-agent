// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packages

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v2"
)

// enableOTelCollectorConfigInDatadogYAML adds otelcollector.enabled and agent_ipc defaults to the given datadog.yaml path
// nolint:unused // Called only from platform-specific code/contexts
func enableOTelCollectorConfigInDatadogYAML(ctx HookContext, datadogYamlPath string) (err error) {
	span, _ := ctx.StartSpan("enable_otelcollector_config_in_datadog_yaml")
	defer func() { span.Finish(err) }()

	data, err := os.ReadFile(datadogYamlPath)
	if err != nil {
		return fmt.Errorf("failed to read datadog.yaml: %w", err)
	}
	var existing map[string]any
	if err := yaml.Unmarshal(data, &existing); err != nil {
		return fmt.Errorf("failed to parse datadog.yaml: %w", err)
	}
	if existing == nil {
		existing = map[string]any{}
	}
	existing["otelcollector"] = map[string]any{"enabled": true}
	existing["agent_ipc"] = map[string]any{"port": 5009, "config_refresh_interval": 60}
	updated, err := yaml.Marshal(existing)
	if err != nil {
		return fmt.Errorf("failed to serialize datadog.yaml: %w", err)
	}
	return os.WriteFile(datadogYamlPath, updated, 0o640) // Permissions shouldn't change as the file exists
}

// disableOtelCollectorConfigCommon removes otelcollector and agent_ipc from the given datadog.yaml path
// nolint:unused // Called only from platform-specific code/contexts
func disableOtelCollectorConfigCommon(datadogYamlPath string) error {
	data, err := os.ReadFile(datadogYamlPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read datadog.yaml: %w", err)
	}
	var existing map[string]any
	if err := yaml.Unmarshal(data, &existing); err != nil {
		return fmt.Errorf("failed to parse datadog.yaml: %w", err)
	}
	delete(existing, "otelcollector")
	delete(existing, "agent_ipc")
	updated, err := yaml.Marshal(existing)
	if err != nil {
		return fmt.Errorf("failed to serialize datadog.yaml: %w", err)
	}
	return os.WriteFile(datadogYamlPath, updated, 0o600)
}

// writeOTelConfigCommon creates otel-config.yaml from a template by substituting api_key and site found in datadog.yaml
// If preserveIfExists is true and outPath already exists, the function returns without writing.
// nolint:unused // Called only from platform-specific code/contexts
func writeOTelConfigCommon(ctx HookContext, datadogYamlPath, templatePath, outPath string, preserveIfExists bool, mode os.FileMode) (err error) {
	span, _ := ctx.StartSpan("write_otel_config_common")
	defer func() { span.Finish(err) }()

	if preserveIfExists {
		if _, err := os.Stat(outPath); err == nil {
			return nil
		}
	}

	data, err := os.ReadFile(datadogYamlPath)
	if err != nil {
		return fmt.Errorf("failed to read datadog.yaml: %w", err)
	}
	var cfg map[string]any
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("failed to parse datadog.yaml: %w", err)
	}
	apiKey, _ := cfg["api_key"].(string)
	site, _ := cfg["site"].(string)

	templateData, err := os.ReadFile(templatePath)
	if err != nil {
		return fmt.Errorf("failed to read otel-config template: %w", err)
	}
	content := string(templateData)
	if apiKey != "" {
		content = strings.ReplaceAll(content, "${env:DD_API_KEY}", apiKey)
	}
	if site != "" {
		content = strings.ReplaceAll(content, "${env:DD_SITE}", site)
	}
	return os.WriteFile(outPath, []byte(content), mode)
}
