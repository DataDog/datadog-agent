// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package receivers

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Settings is the source-named routing table shared by YAML and Helm renderers.
// Collection switches are deliberately not enabled. Health uses dd_url, not an
// invented health-platform override. Log-pipeline consumers use logs_dd_url.
func (p Plan) Settings() map[string]any {
	s := map[string]any{"remote_configuration.enabled": p.RemoteConfig == "native"}
	if p.Endpoint == "" {
		s["site"] = p.Site
		return s
	}
	s["dd_url"] = p.Endpoint
	for _, k := range []string{"apm_config.apm_dd_url", "apm_config.telemetry.dd_url", "process_config.process_dd_url", "orchestrator_explorer.orchestrator_dd_url"} {
		s[k] = p.Endpoint
	}
	// These proxy overrides are full URLs, used verbatim by the trace loader
	// and transports. Paths match pkg/trace/config/endpoints.go, not origins.
	for key, path := range map[string]string{
		"apm_config.profiling_dd_url":            "/api/v2/profile",
		"apm_config.debugger_dd_url":             "/api/v2/logs",
		"apm_config.debugger_diagnostics_dd_url": "/api/v2/debugger",
		"apm_config.symdb_dd_url":                "/api/v2/debugger",
	} {
		s[key] = p.Endpoint + path
	}
	// These default-enabled proxies have independent native destinations.
	// EVP expects a domain suffix, not an intake URL. Until their protocols are
	// supported, custom-intake policy explicitly disables (and owns) them.
	s["evp_proxy_config.enabled"] = false
	s["ol_proxy_config.enabled"] = false
	for _, prefix := range []string{"logs_config", "container_image", "container_lifecycle", "sbom", "agent_telemetry"} {
		s[prefix+".logs_dd_url"] = p.Endpoint
		if prefix != "logs_config" {
			s[prefix+".dd_url"] = p.Endpoint
		}
		s[prefix+".logs_no_ssl"] = strings.HasPrefix(p.Endpoint, "http:")
	}
	s["logs_config.force_use_http"] = true
	return s
}

var aliases = map[string]string{
	"DD_URL": "dd_url", "DD_PROCESS_AGENT_URL": "process_config.process_dd_url", "DD_PROCESS_CONFIG_URL": "process_config.process_dd_url",
	"DD_PROCESS_AGENT_PROCESS_DD_URL": "process_config.process_dd_url", "DD_PROCESS_AGENT_ORCHESTRATOR_DD_URL": "process_config.orchestrator_dd_url",
	"DD_PROCESS_AGENT_ADDITIONAL_ENDPOINTS": "process_config.additional_endpoints",
	"DD_PROCESS_ADDITIONAL_ENDPOINTS":       "process_config.additional_endpoints",
	"DD_ORCHESTRATOR_URL":                   "orchestrator_explorer.orchestrator_dd_url",
	"DD_ORCHESTRATOR_ADDITIONAL_ENDPOINTS":  "orchestrator_explorer.orchestrator_additional_endpoints",
}

// EnvName is the canonical documented environment spelling for a setting.
var envNames = map[string]string{
	// apm_config.yaml explicitly registers DD_APM_ aliases (not DD_APM_CONFIG_).
	"apm_config.apm_dd_url":                  "DD_APM_DD_URL",
	"apm_config.profiling_dd_url":            "DD_APM_PROFILING_DD_URL",
	"apm_config.debugger_dd_url":             "DD_APM_DEBUGGER_DD_URL",
	"apm_config.debugger_diagnostics_dd_url": "DD_APM_DEBUGGER_DIAGNOSTICS_DD_URL",
	"apm_config.symdb_dd_url":                "DD_APM_SYMDB_DD_URL",
	"apm_config.telemetry.dd_url":            "DD_APM_TELEMETRY_DD_URL",
}

func EnvName(key string) string {
	if name, ok := envNames[key]; ok {
		return name
	}
	return "DD_" + strings.ToUpper(strings.ReplaceAll(key, ".", "_"))
}

// Owns identifies only destination/credential/trust settings within known Agent
// sender sections. URLs in checks, config providers and kubelet are not routes.
func Owns(key string) bool {
	for _, root := range []string{"api_key", "app_key", "site", "dd_url", "additional_endpoints", "skip_ssl_validation", "api_key_file"} {
		if key == root || strings.HasPrefix(key, root+".") {
			return true
		}
	}

	for _, prefix := range []string{"remote_configuration.", "observability_pipelines_worker.", "vector.", "evp_proxy_config.", "ol_proxy_config."} {
		if strings.HasPrefix(key, prefix) || key == strings.TrimSuffix(prefix, ".") {
			return true
		}
	}
	parts := strings.Split(key, ".")
	if len(parts) < 2 {
		return false
	}
	switch parts[0] {
	case "logs_config", "apm_config", "process_config", "orchestrator_explorer", "container_image", "container_lifecycle", "sbom", "agent_telemetry":
		for _, part := range parts[1:] {
			if strings.Contains(part, "dd_url") || strings.Contains(part, "additional_endpoints") || strings.Contains(part, "api_key") || part == "logs_no_ssl" || part == "dev_mode_no_ssl" || part == "force_use_http" || part == "force_use_tcp" || part == "use_http" || part == "use_tcp" || part == "no_ssl" {
				return true
			}
		}
	}
	return false
}

var unsupportedSections = []string{"runtime_security_config", "compliance_config", "security_agent", "network_devices", "network_path", "synthetics", "private_action_runner", "external_metrics_provider", "autoscaling", "database_monitoring", "internal_profiling", "multi_region_failover", "fips"}

// Unsupported sections have independent backend behavior not covered by this
// renderer. Explicit false is harmless; enabled/opaque input must not escape.
func Unsupported(key string, value any) bool {
	for _, prefix := range unsupportedSections {
		if key == prefix || strings.HasPrefix(key, prefix+".") {
			return value != false
		}
	}
	return (key == "apm_config.profiling.enabled" || strings.Contains(key, ".internal_profiling")) && value != false
}

// EnvKey resolves routing aliases, including the process agent's older names.
func EnvKey(name string) string {
	for _, prefix := range unsupportedSections {
		start := EnvName(prefix) + "_"
		if strings.HasPrefix(name, start) {
			return prefix + "." + strings.ToLower(strings.TrimPrefix(name, start))
		}
	}
	for key, canonical := range envNames {
		if name == canonical {
			return key
		}
	}
	if strings.HasPrefix(name, "DD_APM_") && !strings.HasPrefix(name, "DD_APM_CONFIG_") {
		return "apm_config." + strings.ToLower(strings.TrimPrefix(name, "DD_APM_"))
	}
	if key, ok := aliases[name]; ok {
		return key
	}
	for _, prefix := range []string{"remote_configuration", "observability_pipelines_worker", "vector", "evp_proxy_config", "ol_proxy_config", "runtime_security_config", "compliance_config", "security_agent", "network_devices", "network_path", "private_action_runner", "external_metrics_provider", "internal_profiling", "logs_config", "apm_config", "process_config", "orchestrator_explorer", "container_image", "container_lifecycle", "sbom", "agent_telemetry"} {
		start := EnvName(prefix) + "_"
		if strings.HasPrefix(name, start) {
			return prefix + "." + strings.ToLower(strings.TrimPrefix(name, start))
		}
	}
	return strings.ToLower(strings.TrimPrefix(name, "DD_"))
}

// ParseConfig returns a validation-only projection of dotted/nested keys. Never
// serialize it as Agent configuration: mapping leaves contain literal dictionary
// keys that must retain their case and dots. Errors never include raw values.
func ParseConfig(raw string) (map[string]any, error) {
	result := map[string]any{}
	if strings.TrimSpace(raw) == "" {
		return result, nil
	}
	var doc yaml.Node
	dec := yaml.NewDecoder(strings.NewReader(raw))
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("Agent config: invalid YAML")
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("Agent config: expected one YAML document")
	}
	if len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("Agent config must be a mapping")
	}
	seen := map[string]bool{}
	var walk func(*yaml.Node, string) error
	walk = func(n *yaml.Node, prefix string) error {
		if n.Kind != yaml.MappingNode {
			return fmt.Errorf("%s: expected mapping", prefix)
		}
		for i := 0; i < len(n.Content); i += 2 {
			k, v := n.Content[i], n.Content[i+1]
			if k.Tag != "!!str" || k.Value == "<<" {
				return fmt.Errorf("Agent config: string keys required; YAML merges are unsupported")
			}
			key := strings.ToLower(k.Value)
			if prefix != "" {
				key = prefix + "." + key
			}
			if seen[key] {
				return fmt.Errorf("%s: duplicate dotted/nested setting", key)
			}
			seen[key] = true
			if v.Kind == yaml.MappingNode {
				if len(v.Content) == 0 {
					result[key] = map[string]any{}
				}
				if err := walk(v, key); err != nil {
					return err
				}
				continue
			}
			if err := rejectAliases(v); err != nil {
				return err
			}
			var value any
			if err := v.Decode(&value); err != nil {
				return fmt.Errorf("%s: invalid value", key)
			}
			result[key] = value
		}
		return nil
	}
	if err := walk(doc.Content[0], ""); err != nil {
		return nil, err
	}
	return result, nil
}
func rejectAliases(n *yaml.Node) error {
	if n.Kind == yaml.AliasNode || n.Value == "<<" && n.Tag == "!!merge" {
		return fmt.Errorf("Agent config: YAML aliases/merges are unsupported")
	}
	for _, c := range n.Content {
		if err := rejectAliases(c); err != nil {
			return err
		}
	}
	return nil
}

// ValidateConfig rejects competing raw routes before any secret lookup/mutation.
func ValidateConfig(raw string) (map[string]any, error) {
	m, err := ParseConfig(raw)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if Owns(key) {
			return nil, fmt.Errorf("%s: owned by agent.receiver; remove the raw destination/credential setting", key)
		}
		if Unsupported(key, m[key]) {
			return nil, fmt.Errorf("%s: backend feature unsupported by explicit receiver routing", key)
		}
	}
	return m, nil
}
