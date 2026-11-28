// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package gpu contains GPU Monitoring installation logic
package gpu

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/setup/common"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/setup/config"
)

const (
	// defaultAgentVersion is a major version tag, so every install run pulls
	// the latest 7.x Agent. Customers can pin it with DD_AGENT_MINOR_VERSION.
	defaultAgentVersion = "7"
	// databricksInjectorVersion must be 0.73.0 or later: older injectors look
	// for the C tracer in apm-library-c instead of datadog-apm-library-c.
	databricksInjectorVersion = "0.74.1-1"
	// defaultCTracerVersion is a major version tag, so every install run pulls
	// the latest 0.x release of dd-trace-c.
	defaultCTracerVersion = "0"
	// envCTracerVersion lets customers pin the C tracer version, e.g. 0.40.0
	envCTracerVersion = "DD_C_TRACER_VERSION"

	gpuRestartDelay   = 60 * time.Second
	gpuRestartLogFile = "/var/log/datadog-gpu-restart"

	sparkEnvPath       = "/databricks/spark/conf/spark-env.sh"
	sparkEnvBlockStart = "# BEGIN Datadog GPU Monitoring"
	sparkEnvBlockEnd   = "# END Datadog GPU Monitoring"
)

// fullVersionWithoutRevision matches versions like 0.40.0, which are
// published with a package revision suffix (0.40.0-1).
var fullVersionWithoutRevision = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

// clusterNameRegex matches the characters replaced in cluster name tags, like
// the Data Jobs Monitoring script does.
var clusterNameRegex = regexp.MustCompile(`[^a-zA-Z0-9_:.-]+`)

var (
	workspaceNameRegex = regexp.MustCompile(`[^\p{L}\p{N}_:./-]+`)
	dedupeUnderscores  = regexp.MustCompile(`_+`)
)

// SetupDatabricksGPU sets up GPU Monitoring on Databricks: GPU metrics and
// GPU traces from the C tracer (dd-trace-c), which the APM injector loads into
// processes. It does not set up Data Jobs Monitoring.
func SetupDatabricksGPU(s *common.Setup) error {
	if os.Getenv("DD_NO_AGENT_INSTALL") != "true" {
		agentVersion := agentVersion(s)
		s.Out.WriteString(fmt.Sprintf("Installing the Datadog Agent version %s\n", agentVersion))
		s.Packages.Install(common.DatadogAgentPackage, agentVersion)
	}
	s.Packages.Install(common.DatadogAPMInjectPackage, databricksInjectorVersion)
	version := cTracerVersion()
	s.Out.WriteString(fmt.Sprintf("Installing the Datadog C tracer version %s\n", version))
	s.Packages.Install(common.DatadogAPMLibraryCPackage, version)
	s.Span.SetTag("c_tracer_version", version)

	hostname, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("failed to get hostname: %w", err)
	}
	s.Config.DatadogYAML.Hostname = hostname
	s.Config.DatadogYAML.ExpectedTagsDuration = "10m"
	s.Config.DatadogYAML.ProcessConfig.ExpvarPort = 6063 // avoid port conflict on 6062

	setupDatabricksHostTags(s)
	setupDatabricksEnv(s, paths.DatadogDataDir)
	setupGPUCheck(s)
	setupTracerConfig(s)
	if err := setupTracerEnv(s, sparkEnvPath); err != nil {
		return fmt.Errorf("failed to set up the C tracer environment: %w", err)
	}
	return nil
}

// setupTracerConfig writes the tracer settings to application_monitoring.yaml,
// like the Data Jobs Monitoring script. The injector and the C tracer don't
// read it yet, so setupTracerEnv also exports them in spark-env.sh.
func setupTracerConfig(s *common.Setup) {
	s.Out.WriteString("Enabling the C tracer and GPU tracing in application_monitoring.yaml\n")
	cfg := config.APMConfigurationDefault{}
	for _, kv := range tracerEnv() {
		switch kv[0] {
		case "DD_INJECT_NATIVE":
			cfg.InjectNative = kv[1]
		case "DD_GPU_ENABLED":
			cfg.GPUEnabled = config.BoolToPtr(kv[1] == "true")
		case "DD_TRACE_LOG_FILE":
			cfg.TraceLogFile = kv[1]
		case "DD_TRACE_LOG_LEVEL":
			cfg.TraceLogLevel = kv[1]
		case "DD_SERVICE":
			cfg.Service = kv[1]
		case "DD_ENV":
			cfg.Env = kv[1]
		}
	}
	s.Config.ApplicationMonitoringYAML = &config.ApplicationMonitoringConfig{Default: cfg}
}

// setupTracerEnv exports the injector and C tracer settings in spark-env.sh,
// which Spark sources before starting the driver and executors. The injector
// and the C tracer only read their settings from the process environment.
func setupTracerEnv(s *common.Setup, path string) error {
	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		s.Out.WriteString(fmt.Sprintf("Skipping the C tracer environment: %s not found\n", filepath.Dir(path)))
		return nil
	}
	content, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	block := []string{sparkEnvBlockStart}
	var keys []string
	for _, kv := range tracerEnv() {
		block = append(block, fmt.Sprintf("export %s=%s", kv[0], shellQuote(kv[1])))
		keys = append(keys, kv[0])
	}
	s.Out.WriteString(fmt.Sprintf("Exporting %s in %s\n", strings.Join(keys, ", "), path))
	block = append(block, sparkEnvBlockEnd)
	updated := removeSparkEnvBlock(string(content))
	if updated != "" && !strings.HasSuffix(updated, "\n") {
		updated += "\n"
	}
	updated += strings.Join(block, "\n") + "\n"
	return os.WriteFile(path, []byte(updated), 0644)
}

// tracerEnv returns the environment of traced processes: the injector loads
// the C tracer (native plugins are off by default) and the C tracer enables
// GPU monitoring (off by default). DD_SERVICE, DD_ENV and the C tracer log
// settings are passed through when set: without DD_SERVICE the C tracer names
// the service after the process, and without DD_TRACE_LOG_FILE (e.g. stderr
// or a path) it doesn't log.
func tracerEnv() [][2]string {
	vars := [][2]string{
		{"DD_INJECT_NATIVE", "always"},
		{"DD_GPU_ENABLED", "true"},
	}
	for _, key := range []string{"DD_SERVICE", "DD_ENV", "DD_TRACE_LOG_FILE", "DD_TRACE_LOG_LEVEL"} {
		if value := os.Getenv(key); value != "" {
			vars = append(vars, [2]string{key, value})
		}
	}
	return vars
}

// removeSparkEnvBlock removes the block written by a previous run.
func removeSparkEnvBlock(content string) string {
	start := strings.Index(content, sparkEnvBlockStart)
	if start < 0 {
		return content
	}
	end := strings.Index(content[start:], sparkEnvBlockEnd)
	if end < 0 {
		return content[:start]
	}
	return content[:start] + strings.TrimPrefix(content[start+end+len(sparkEnvBlockEnd):], "\n")
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

// setupGPUCheck enables the GPU check and schedules an Agent restart, since
// NVML is initialized after the Databricks init script runs.
func setupGPUCheck(s *common.Setup) {
	s.Out.WriteString("Setting up GPU monitoring\n")
	s.Config.DatadogYAML.GPUCheck.Enabled = config.BoolToPtr(true)
	s.DelayedAgentRestartConfig.Scheduled = true
	s.DelayedAgentRestartConfig.Delay = gpuRestartDelay
	s.DelayedAgentRestartConfig.LogFile = gpuRestartLogFile
}

// setupDatabricksHostTags sets the Databricks host tags, with the same keys
// as the Data Jobs Monitoring script, and adds DD_TAGS and DD_EXTRA_TAGS.
func setupDatabricksHostTags(s *common.Setup) {
	addTagFromEnv(s, "DB_CLUSTER_ID", "databricks_cluster_id", nil)
	addTagFromEnv(s, "DB_CLUSTER_ID", "cluster_id", nil)
	sanitizeClusterName := func(v string) string { return clusterNameRegex.ReplaceAllString(v, "_") }
	addTagFromEnv(s, "DB_CLUSTER_NAME", "databricks_cluster_name", sanitizeClusterName)
	addTagFromEnv(s, "DB_CLUSTER_NAME", "cluster_name", sanitizeClusterName)
	addTagFromEnv(s, "DATABRICKS_WORKSPACE", "databricks_workspace", nil)
	addTagFromEnv(s, "WORKSPACE_URL", "workspace_url", nil)
	addTagFromEnv(s, "DB_INSTANCE_TYPE", "databricks_instance_type", nil)
	addTagFromEnv(s, "DB_IS_JOB_CLUSTER", "databricks_is_job_cluster", nil)
	addTagFromEnv(s, "DATABRICKS_RUNTIME_VERSION", "databricks_runtime", nil)
	if os.Getenv("DB_IS_DRIVER") == "TRUE" {
		s.Config.DatadogYAML.Tags = append(s.Config.DatadogYAML.Tags, "spark_node:driver")
	} else {
		s.Config.DatadogYAML.Tags = append(s.Config.DatadogYAML.Tags, "spark_node:worker")
	}

	// DD_TAGS and DD_EXTRA_TAGS are separated by commas or spaces
	isSeparator := func(r rune) bool { return r == ',' || r == ' ' }
	s.Config.DatadogYAML.Tags = append(s.Config.DatadogYAML.Tags, strings.FieldsFunc(os.Getenv("DD_TAGS"), isSeparator)...)
	s.Config.DatadogYAML.ExtraTags = append(s.Config.DatadogYAML.ExtraTags, strings.FieldsFunc(os.Getenv("DD_EXTRA_TAGS"), isSeparator)...)
}

// setupDatabricksEnv defaults the Agent env to the normalized workspace name,
// like the Data Jobs Monitoring script, unless DD_ENV is set or a previous
// install already configured an env.
func setupDatabricksEnv(s *common.Setup, configDir string) {
	workspace := normalizeWorkspaceName(os.Getenv("DATABRICKS_WORKSPACE"))
	if workspace == "" || os.Getenv("DD_ENV") != "" || config.ReadCurrentEnv(configDir) != "" {
		return
	}
	s.Out.WriteString(fmt.Sprintf("Setting the Agent env to the workspace name %s\n", workspace))
	s.Config.DatadogYAML.Env = workspace
}

func addTagFromEnv(s *common.Setup, envKey, tagKey string, normalize func(string) string) {
	value, ok := os.LookupEnv(envKey)
	if !ok {
		return
	}
	if normalize != nil {
		value = normalize(value)
	}
	s.Config.DatadogYAML.Tags = append(s.Config.DatadogYAML.Tags, tagKey+":"+value)
}

// agentVersion returns the Agent version requested with DD_AGENT_MAJOR_VERSION
// and DD_AGENT_MINOR_VERSION (e.g. DD_AGENT_MINOR_VERSION=86.0), or the latest
// Agent 7 when none is requested.
func agentVersion(s *common.Setup) string {
	if s.Env == nil {
		return defaultAgentVersion
	}
	version := s.Env.GetAgentVersion()
	if version == "latest" {
		return defaultAgentVersion
	}
	return version
}

func cTracerVersion() string {
	version := os.Getenv(envCTracerVersion)
	if version == "" {
		return defaultCTracerVersion
	}
	if fullVersionWithoutRevision.MatchString(version) {
		return version + "-1"
	}
	return version
}

// normalizeWorkspaceName normalizes a workspace name to match Python's normalize_tags behavior.
// It must stay identical to normalizeWorkspaceName in the djm package:
// 1. Convert to all lowercase unicode string
// 2. Trim leading/trailing quotes
// 3. Convert bad characters to underscores
// 4. Dedupe contiguous underscores
// 5. Remove initial underscores/digits such that the string starts with an alpha char
// 6. Truncate to 200 characters
// 7. Strip trailing underscores
func normalizeWorkspaceName(value string) string {
	if value == "" {
		return ""
	}
	normalized := strings.ToLower(value)
	normalized = strings.Trim(normalized, "\"'")
	normalized = workspaceNameRegex.ReplaceAllString(normalized, "_")
	normalized = dedupeUnderscores.ReplaceAllString(normalized, "_")

	if len(normalized) > 0 && (normalized[0] == '_' || normalized[0] == ':') {
		normalized = strings.TrimLeft(normalized, "_")
		if len(normalized) > 0 && unicode.IsDigit([]rune(normalized)[0]) {
			normalized = strings.TrimLeft(normalized, "_:0123456789")
		}
	}

	if len(normalized) > 200 {
		normalized = normalized[:200]
	}
	return strings.TrimRight(normalized, "_")
}
