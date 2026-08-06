// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package diagnostic provides optional startup-time diagnostic logging for
// serverless-init. Enable by setting DD_SERVERLESS_DIAGNOSTIC_INFO=true.
//
// When enabled, logs all environment variables (with secrets masked), key
// agent configuration values, and mode/origin detection results on startup.
//
// WARNING: Output may contain sensitive values. Do not enable in production.
package diagnostic

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/cloudservice"
	"github.com/DataDog/datadog-agent/cmd/serverless-init/mode"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	servertags "github.com/DataDog/datadog-agent/pkg/serverless/tags"
	pkgversion "github.com/DataDog/datadog-agent/pkg/version"
	"github.com/DataDog/datadog-agent/pkg/util/uuid"
)

const diagnosticEnvVar = "DD_SERVERLESS_DIAGNOSTIC_INFO"

// secretKeywords are substrings whose presence in an env var name triggers masking.
var secretKeywords = []string{"API_KEY", "SECRET", "PASSWORD", "TOKEN"}

// envDescriptions maps known environment variable names to a short human-readable
// description shown alongside the value in diagnostic output.
var envDescriptions = map[string]string{
	// Datadog configuration
	"DD_API_KEY":                          "Datadog API key (masked)",
	"DD_SITE":                             "Datadog intake site (e.g. datadoghq.com, datadoghq.eu)",
	"DD_SERVICE":                          "Service name used for tagging all telemetry",
	"DD_ENV":                              "Deployment environment tag (dev/staging/prod)",
	"DD_VERSION":                          "App version tag for deployments",
	"DD_LOGS_ENABLED":                     "Enable log collection and forwarding to Datadog",
	"DD_APM_ENABLED":                      "Enable APM trace collection",
	"DD_TRACE_ENABLED":                    "Enable distributed tracing via ddtrace",
	"DD_HOSTNAME":                         "Override the hostname reported to Datadog",
	"DD_REMOTE_CONFIGURATION_ENABLED":     "Enable Remote Configuration (live config updates from Datadog UI)",
	"DD_SERVERLESS_DIAGNOSTIC_INFO":       "Enable this diagnostic output on startup",
	"DD_INSTRUMENTATION_TELEMETRY_ENABLED": "Send ddtrace instrumentation telemetry to Datadog",
	// Cloud Run platform
	"K_SERVICE":                 "Cloud Run service name",
	"K_REVISION":                "Current Cloud Run revision (e.g. myservice-00005-abc)",
	"K_CONFIGURATION":           "Cloud Run configuration name",
	"PORT":                      "Port the container must listen on",
	"CLOUD_RUN_TIMEOUT_SECONDS": "Maximum request handling time before Cloud Run terminates the instance",
	// Python base image
	"PYTHON_VERSION": "Python runtime version injected by the python base image",
	"PYTHON_SHA256":  "SHA256 checksum of the Python release tarball",
	"GPG_KEY":        "GPG key used to verify the Python release",
	// Node base image
	"NODE_VERSION": "Node.js runtime version injected by the node base image",
	// Standard
	"PATH": "Executable search path",
	"HOME": "Home directory of the running user",
	"PWD":  "Current working directory",
	"LANG": "System locale",
}

// diag writes a single SERVERLESS_DIAGNOSTIC line to stdout. Using fmt.Fprintf
// (not log.Infof) ensures output is visible in Cloud Logging even when called
// before the Fx logger is initialized.
func diag(format string, args ...interface{}) {
	fmt.Fprintf(os.Stdout, "[SERVERLESS_DIAGNOSTIC] "+format+"\n", args...)
}

// LogIfEnabled logs diagnostic startup information when DD_SERVERLESS_DIAGNOSTIC_INFO=true.
// Call once after DetectMode and GetCloudServiceType, before fxutil.OneShot.
func LogIfEnabled(modeConf mode.Conf, cs cloudservice.CloudService) {
	if strings.ToLower(os.Getenv(diagnosticEnvVar)) != "true" {
		return
	}

	agentVersion := "(unknown)"
	if v, err := pkgversion.Agent(); err == nil {
		agentVersion = v.String()
	}
	commit := pkgversion.Commit
	if commit == "" {
		commit = "(unknown)"
	}
	// Agent version = underlying agent release (e.g. 7.83.0-dev for branch builds, 7.81.2 for releases).
	// The official serverless-init image tag (e.g. 1.10.2) maps 1:1 to an agent release; Datadog's CI
	// sets both. To correlate: gcr.io/datadoghq/serverless-init:1.10.2 was built from agent 7.81.2.
	diag("agent version:           %s  # Datadog Agent release this binary is built from; official serverless-init tag (e.g. 1.10.2) maps 1:1 to this", agentVersion)
	diag("commit:                  %s  # git SHA of the source — for branch builds this identifies the exact code running", commit)
	diag("serverless_init_version: %s  # Docker image tag (e.g. 1.10.0); injected at build time via -ldflags (pkg/serverless/tags.currentExtensionVersion)", servertags.GetExtensionVersion())
	diag("started:      %s", time.Now().UTC().Format(time.RFC3339))
	diag("mode:         %s", modeConf.LoggerName)
	diag("sidecar:      %v", modeConf.SidecarMode)
	if modeConf.SidecarMode {
		diag("deployment_model: sidecar        # serverless-init runs as a sidecar; the app is a separate container")
		diag("wrapped_command:  (none — sidecar mode)")
	} else {
		wrappedCmd := strings.Join(os.Args[1:], " ")
		if wrappedCmd == "" {
			wrappedCmd = "(none)"
		}
		diag("deployment_model: init-container  # serverless-init wraps the app command")
		diag("wrapped_command:  %s", wrappedCmd)
	}
	diag("origin:       %s  # cloud platform detected at runtime (cloudrun, cloudrunfunctions, azure, etc.)", cs.GetOrigin())
	diag("build_tags:   serverless,otlp  # compile-time flags: 'serverless'=serverless code paths + no zlib/zstd, 'otlp'=OTLP trace/metric endpoint")
	diag("runtime:      %s  # language runtime inferred from base image env vars", detectRuntime())
	diag("revision:     %s  # current deployment revision", firstEnv("K_REVISION", "WEBSITE_SITE_NAME", "CONTAINER_APP_NAME"))
	diag("ddtrace:      %s  # ddtrace library version (set DD_TRACE_VERSION env var to surface this)", firstEnv("DD_TRACE_VERSION", "DD_TRACER_VERSION"))

	envVars := os.Environ()
	sort.Strings(envVars)
	diag("--- env vars (%d total) ---", len(envVars))
	for _, e := range envVars {
		masked := maskSecret(e)
		key := strings.SplitN(e, "=", 2)[0]
		if desc, ok := envDescriptions[key]; ok {
			diag("env: %-45s  # %s", masked, desc)
		} else {
			diag("env: %s", masked)
		}
	}

	cfg := pkgconfigsetup.Datadog()
	diag("--- agent config ---")
	diag("config: dd_site=%s                       # Datadog intake site", cfg.GetString("site"))
	diag("config: logs_enabled=%v                  # log collection active", cfg.GetBool("logs_enabled"))
	diag("config: apm_enabled=%v                  # APM trace collection active", cfg.GetBool("apm_config.enabled"))
	diag("config: serializer_compressor_kind=%s   # metric payload compression; forced 'none' because zstd/zlib not compiled in (serverless build tag)", cfg.GetString("serializer_compressor_kind"))
	diag("config: logs_use_compression=%v         # log payload compression; forced false for same reason", cfg.GetBool("logs_config.use_compression"))
	diag("config: api_key_present=%v              # Datadog API key is configured", cfg.GetString("api_key") != "")

	// --- Inventory pipeline identity ---
	// uuid.GetUUID() returns the gopsutil DMI product_uuid (or random UUID fallback).
	// hostname is always "" in the serverless build (pkg/util/hostname/providers_serverless.go
	// returns "" unconditionally). Subotka accepts UUID-only payloads per PR #22542.
	// Look for this UUID in REDAPL: SELECT * FROM datadog_agent WHERE uuid = '<value below>'
	diag("--- inventory pipeline (Subotka → agentmetadata track → datadog_agent table) ---")
	diag("uuid (agent):        %s  # gopsutil UUID — this is what inventoryagent sends to Subotka; query REDAPL by this value", uuid.GetUUID())
	diag("hostname (payload):  (empty string)  # pkg/util/hostname/providers_serverless.go always returns \"\"; Subotka uses UUID as PK instead")
	diag("ccrid (from tags):   %s  # cloud resource ID for REDAPL join — links datadog_agent row to cloud resource tables", cloudCCRID(cs))
	diag("REDAPL query hint:   SELECT * FROM datadog_agent WHERE agent_version LIKE '7.83.0%%' LIMIT 20")

	enabled := cfg.GetBool("enable_metadata_collection")
	firstDelay := cfg.GetDuration("inventories_first_run_delay")
	minInterval := cfg.GetInt("inventories_min_interval")
	maxInterval := cfg.GetInt("inventories_max_interval")
	diag("enable_metadata_collection: %v  # if false, metadata runner does not start and NO payload is ever sent", enabled)
	diag("inventories_first_run_delay: %v  # payload is suppressed until this time has elapsed since agent start (default: 0)", firstDelay)
	diag("inventories_min_interval:   %ds  # runner polls every N seconds (default: 60)", minInterval)
	diag("inventories_max_interval:   %ds  # payload forced even without change every N seconds (default: 600)", maxInterval)
	if !enabled {
		diag("WARNING: enable_metadata_collection=false — inventoryagent will NOT send anything to Subotka")
	}
}

// cloudCCRID returns the Canonical Cloud Resource ID from the tags already computed by
// cloudservice.GetTags(). These are the same tags used to set uuid.GetUUID in main.go,
// so they match what Subotka will receive.
func cloudCCRID(cs cloudservice.CloudService) string {
	tags := cs.GetTags()
	for _, key := range []string{"gcr.resource_name", "gcrfx.resource_name", "gcrj.resource_name", "resource_id"} {
		if v := tags[key]; v != "" {
			return v
		}
	}
	return fmt.Sprintf("(not found in tags — origin=%s)", cs.GetOrigin())
}

// firstEnv returns the value of the first env var in the list that is set and
// non-empty, or "(unknown)" if none match.
func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return "(unknown)"
}

// detectRuntime infers the language runtime from well-known env vars injected
// by base images (PYTHON_VERSION, NODE_VERSION, etc.).
func detectRuntime() string {
	if v := os.Getenv("PYTHON_VERSION"); v != "" {
		return "python" + v
	}
	if v := os.Getenv("NODE_VERSION"); v != "" {
		return "node" + v
	}
	if v := os.Getenv("JAVA_VERSION"); v != "" {
		return "java" + v
	}
	if v := os.Getenv("DOTNET_VERSION"); v != "" {
		return "dotnet" + v
	}
	if v := os.Getenv("RUBY_VERSION"); v != "" {
		return "ruby" + v
	}
	return "(unknown)"
}

// maskSecret replaces the value portion of a KEY=value pair with *** when the
// key contains a known secret keyword. Non-secret vars are returned unchanged.
func maskSecret(kv string) string {
	parts := strings.SplitN(kv, "=", 2)
	if len(parts) != 2 {
		return kv
	}
	upper := strings.ToUpper(parts[0])
	for _, keyword := range secretKeywords {
		if strings.Contains(upper, keyword) {
			return parts[0] + "=***"
		}
	}
	return kv
}
