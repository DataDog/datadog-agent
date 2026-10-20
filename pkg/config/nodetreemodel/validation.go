// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package nodetreemodel

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/schema"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	knownEnvVarsMu sync.RWMutex
	// knownEnvVars accumulates the environment variables bound by every ntmConfig instance (ie: from both
	// datadog.yaml and system-probe.yaml settings) so that each config can tell which DD_* variables are unknown.
	knownEnvVars = map[string]struct{}{
		// these variables are used by the agent, but not via the Config struct,
		// so must be listed separately.
		"DD_INSIDE_CI":      {},
		"DD_PROXY_HTTP":     {},
		"DD_PROXY_HTTPS":    {},
		"DD_PROXY_NO_PROXY": {},
		// these variables are used by serverless, but not via the Config struct
		"DD_AAS_DOTNET_EXTENSION_VERSION":          {},
		"DD_AAS_EXTENSION_VERSION":                 {},
		"DD_AAS_JAVA_EXTENSION_VERSION":            {},
		"DD_AGENT_PIPE_NAME":                       {},
		"DD_API_KEY_SECRET_ARN":                    {},
		"DD_APM_FLUSH_DEADLINE_MILLISECONDS":       {},
		"DD_APPSEC_ENABLED":                        {},
		"DD_AZURE_APP_SERVICES":                    {},
		"DD_DOGSTATSD_ARGS":                        {},
		"DD_DOGSTATSD_PATH":                        {},
		"DD_DOGSTATSD_WINDOWS_PIPE_NAME":           {},
		"DD_DOTNET_TRACER_HOME":                    {},
		"DD_EXTENSION_PATH":                        {},
		"DD_FLUSH_TO_LOG":                          {},
		"DD_KMS_API_KEY":                           {},
		"DD_INTEGRATIONS":                          {},
		"DD_INTERNAL_NATIVE_LOADER_PATH":           {},
		"DD_INTERNAL_PROFILING_NATIVE_ENGINE_PATH": {},
		"DD_LOGS_INJECTION":                        {},
		"DD_MERGE_XRAY_TRACES":                     {},
		"DD_PROFILER_EXCLUDE_PROCESSES":            {},
		"DD_PROFILING_LOG_DIR":                     {},
		"DD_RUNTIME_METRICS_ENABLED":               {},
		"DD_SERVERLESS_APPSEC_ENABLED":             {},
		"DD_SERVERLESS_FLUSH_STRATEGY":             {},
		"DD_SERVICE":                               {},
		"DD_TRACE_AGENT_ARGS":                      {},
		"DD_TRACE_AGENT_PATH":                      {},
		"DD_TRACE_AGENT_URL":                       {},
		"DD_TRACE_LOG_DIRECTORY":                   {},
		"DD_TRACE_LOG_PATH":                        {},
		"DD_TRACE_METRICS_ENABLED":                 {},
		"DD_TRACE_PIPE_NAME":                       {},
		"DD_TRACE_TRANSPORT":                       {},
		"DD_VERSION":                               {},
		// this variable is used by the Kubernetes leader election mechanism
		"DD_POD_NAME": {},
		// this variable is used by tracers
		"DD_INSTRUMENTATION_TELEMETRY_ENABLED": {},
		// these variables are used by source code integration
		"DD_GIT_COMMIT_SHA":     {},
		"DD_GIT_REPOSITORY_URL": {},
		// signals whether or not ADP is enabled (deprecated)
		"DD_ADP_ENABLED": {},
		// trace-loader socket file descriptors
		"DD_APM_NET_RECEIVER_FD":  {},
		"DD_APM_UNIX_RECEIVER_FD": {},
		"DD_OTLP_CONFIG_GRPC_FD":  {},
		// these variables are consumed by the otel-agent via CLI flags (envflag)
		"DD_SYNC_DELAY":  {},
		"DD_SYNC_TO":     {},
		"DD_CORE_CONFIG": {},
		// this variable is no longer used but is still injected by the Helm chart on Linux for backward
		// compatibility with older agents
		"DD_PROCESS_CONFIG_RUN_IN_CORE_AGENT_ENABLED": {},
	}
)

// Validate checks the configuration for unexpected unicode and unknown DD_* environment variables, and validates it
// against its schema, reverting invalid settings to their default. Every resulting warning is added to the config
// warnings.
func (c *ntmConfig) Validate() {
	c.findUnexpectedUnicode()
	c.findUnknownEnvVars(os.Environ())
	c.validateSchema()
}

// validateSchema validates the configuration against its schema and reverts every invalid setting to its default
func (c *ntmConfig) validateSchema() {
	violations, err := schema.ValidateConfig(c, c.schemaID)
	if err != nil {
		log.Errorf("could not validate config against schema: %s", err)
		return
	}

	c.Lock()
	defer c.Unlock()
	for _, v := range violations {
		if v.SettingKey == "" {
			c.warnings = append(c.warnings, "invalid configuration: "+v.Message)
			continue
		}
		c.warnings = append(c.warnings, fmt.Sprintf("invalid value for '%s', reverting to default. Error: %s", v.SettingKey, v.Message))

		defaultValue := c.leafAtPathFromNode(v.SettingKey, c.defaults).Get()
		newTree, err := c.insertValueIntoTree(v.SettingKey, defaultValue, model.SourceConfigPostInit)
		if err != nil {
			_ = log.ErrorfStackDepth(2, "could not insert value: %s", err)
		} else if newTree != nil {
			// a new node was allocated, merge it into root
			c.root, _ = c.root.Merge(newTree)
		}
	}
}

// registerKnownEnvVars registers environment variables bound to a setting, so they are not reported as unknown
// by Validate. This is global for all instances of nodetreemodel (so both datadog.yaml and system-probe.yaml)
func registerKnownEnvVars(envVars ...string) {
	knownEnvVarsMu.Lock()
	defer knownEnvVarsMu.Unlock()
	for _, envVar := range envVars {
		knownEnvVars[envVar] = struct{}{}
	}
}

// findUnknownEnvVars adds a warning to the config for every DD_* variable from environ that is not known by any
// config instance
func (c *ntmConfig) findUnknownEnvVars(environ []string) {
	knownEnvVarsMu.RLock()
	defer knownEnvVarsMu.RUnlock()

	if c.schemaID != model.CoreSchema {
		// we don't want to validate env vars multiple times since it's the same for all instances of
		// nodetreemodel.
		return
	}

	for _, equality := range environ {
		key, _, _ := strings.Cut(equality, "=")
		if !strings.HasPrefix(key, "DD_") {
			continue
		}
		if _, known := knownEnvVars[key]; !known {
			c.warnings = append(c.warnings, "Unknown environment variable: "+key)
		}
	}
}

// findUnexpectedCodepoints adds a warning to the config for every _unexpected_ unicode codepoint found in the
// given 'value' string. Each warning is prefixed with 'prefix'.
// Unexpected here generally means invisible whitespace and control chars
func (c *ntmConfig) findUnexpectedCodepoints(key string, value string, isSecret bool) {
	for position, r := range value {
		reason := ""
		switch {
		case r == utf8.RuneError:
			reason = "RuneError"
		case r == ' ' || r == '\r' || r == '\n' || r == '\t':
			// These are allowed whitespace
			reason = ""
		case unicode.IsSpace(r):
			reason = "unsupported whitespace"
		case unicode.Is(unicode.Bidi_Control, r):
			reason = "Bidirectional control"
		case unicode.Is(unicode.C, r):
			reason = "Control/surrogate"
		}

		if reason != "" {
			if isSecret {
				value = "<REDACTED SECRET>"
			}
			c.warnings = append(c.warnings, fmt.Sprintf("For key '%s', configuration value string '%s' - Unexpected unicode %s codepoint '%U' detected at byte position %v", key, value, reason, r, position))
		}
	}
}

// findUnexpectedUnicode adds a warning to the config for every unexpected unicode codepoint found in the
// configuration keys and string values
func (c *ntmConfig) findUnexpectedUnicode() {
	var visitElement func(string, interface{}, bool)
	visitElement = func(key string, element interface{}, isSecret bool) {
		switch elementValue := element.(type) {
		case string:

			c.findUnexpectedCodepoints(key, elementValue, isSecret)
		case []string:
			for _, s := range elementValue {
				c.findUnexpectedCodepoints(key, s, isSecret)
			}
		case []interface{}:
			for _, listItem := range elementValue {
				visitElement(key, listItem, isSecret)
			}
		}
	}

	// Read every setting under a single lock: going through Get/GetSource for each key would rebuild the config
	// (which reads the whole environment) twice per setting.
	type setting struct {
		key      string
		value    interface{}
		isSecret bool
	}
	c.maybeRebuild()
	c.RLock()
	keys := c.collectFlattenedKeys()
	slices.Sort(keys)
	settings := make([]setting, 0, len(keys))
	for _, key := range keys {
		leaf := c.leafAtPath(key)
		settings = append(settings, setting{
			key:      key,
			value:    c.getNodeValue(key),
			isSecret: leaf.Source() == model.SourceSecret,
		})
	}
	c.RUnlock()

	for _, s := range settings {
		c.findUnexpectedCodepoints(s.key, fmt.Sprintf("Configuration key string '%s'", s.key), false)
		if s.value != nil {
			visitElement(s.key, s.value, s.isSecret)
		}
	}
}
