// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package djm contains data-jobs-monitoring installation logic
package djm

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/setup/common"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/setup/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	databricksInjectorVersion   = "0.53.0-1"
	databricksJavaTracerVersion = "1.58.0-1"
	databricksAgentVersion      = "7.74.0-1"
	gpuIntegrationRestartDelay  = 60 * time.Second
	restartLogFile              = "/var/log/datadog-gpu-restart"
)

var (
	jobNameRegex       = regexp.MustCompile(`[,\']+`)
	clusterNameRegex   = regexp.MustCompile(`[^a-zA-Z0-9_:.-]+`)
	workspaceNameRegex = regexp.MustCompile(`[^\p{L}\p{N}_:./-]+`)
	dedupeUnderscores  = regexp.MustCompile(`_+`)
	driverLogs         = []config.IntegrationConfigLogs{
		{
			Type:                   "file",
			Path:                   "/databricks/driver/logs/*.log",
			Source:                 "driver_logs",
			Service:                "databricks",
			AutoMultiLineDetection: config.BoolToPtr(true),
		},
		{
			Type:                   "file",
			Path:                   "/databricks/driver/logs/stderr",
			Source:                 "driver_stderr",
			Service:                "databricks",
			AutoMultiLineDetection: config.BoolToPtr(true),
		},
		{
			Type:    "file",
			Path:    "/databricks/driver/logs/stdout",
			Source:  "driver_stdout",
			Service: "databricks",
			LogProcessingRules: []config.LogProcessingRule{
				{Type: "multi_line", Name: "logger_multiline", Pattern: "(^\\+[-+]+\\n(\\|.*\\n)+\\+[-+]+$)|^(ERROR|INFO|DEBUG|WARN|CRITICAL|NOTSET|Traceback)"},
			},
			AutoMultiLineDetection: config.BoolToPtr(true),
		},
	}
	driverLogsStandardAccessMode = []config.IntegrationConfigLogs{
		{
			Type:                   "file",
			Path:                   "/var/log/databricks_privileged/*.log",
			Source:                 "driver_logs",
			Service:                "databricks",
			AutoMultiLineDetection: config.BoolToPtr(true),
		},
		{
			Type:                   "file",
			Path:                   "/var/log/databricks_privileged/stderr",
			Source:                 "driver_stderr",
			Service:                "databricks",
			AutoMultiLineDetection: config.BoolToPtr(true),
		},
		{
			Type:    "file",
			Path:    "/var/log/databricks_privileged/stdout",
			Source:  "driver_stdout",
			Service: "databricks",
			LogProcessingRules: []config.LogProcessingRule{
				{Type: "multi_line", Name: "logger_multiline", Pattern: "(^\\+[-+]+\\n(\\|.*\\n)+\\+[-+]+$)|^(ERROR|INFO|DEBUG|WARN|CRITICAL|NOTSET|Traceback)"},
			},
			AutoMultiLineDetection: config.BoolToPtr(true),
		},
	}
	workerLogs = []config.IntegrationConfigLogs{
		{
			Type:                   "file",
			Path:                   "/databricks/spark/work/*/*/stderr",
			Source:                 "worker_stderr",
			Service:                "databricks",
			AutoMultiLineDetection: config.BoolToPtr(true),
		},
		{
			Type:                   "file",
			Path:                   "/databricks/spark/work/*/*/stdout",
			Source:                 "worker_stdout",
			Service:                "databricks",
			AutoMultiLineDetection: config.BoolToPtr(true),
		},
	}
	workerLogsStandardAccessMode = []config.IntegrationConfigLogs{
		{
			Type:                   "file",
			Path:                   "/var/log/databricks_privileged/*/*/stderr",
			Source:                 "worker_stderr",
			Service:                "databricks",
			AutoMultiLineDetection: config.BoolToPtr(true),
		},
		{
			Type:                   "file",
			Path:                   "/var/log/databricks_privileged/*/*/stdout",
			Source:                 "worker_stdout",
			Service:                "databricks",
			AutoMultiLineDetection: config.BoolToPtr(true),
		},
	}
	tracerConfigDatabricks = config.APMConfigurationDefault{
		DataJobsEnabled:     config.BoolToPtr(true),
		IntegrationsEnabled: config.BoolToPtr(false),
	}
)

// SetupDatabricks sets up the Databricks environment
func SetupDatabricks(s *common.Setup) error {
	if os.Getenv("DD_NO_AGENT_INSTALL") != "true" {
		s.Packages.Install(common.DatadogAgentPackage, databricksAgentVersion)
	}
	s.Packages.Install(common.DatadogAPMInjectPackage, databricksInjectorVersion)
	s.Packages.Install(common.DatadogAPMLibraryJavaPackage, databricksJavaTracerVersion)

	s.Out.WriteString("Applying specific Data Jobs Monitoring config\n")
	hostname, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("failed to get hostname: %w", err)
	}
	s.Config.DatadogYAML.Hostname = hostname
	s.Config.DatadogYAML.DJM.Enabled = config.BoolToPtr(true)
	s.Config.DatadogYAML.ExpectedTagsDuration = "10m"
	s.Config.DatadogYAML.ProcessConfig.ExpvarPort = 6063 // avoid port conflict on 6062

	if os.Getenv("DD_TRACE_DEBUG") == "true" {
		s.Out.WriteString("Enabling Datadog Java Tracer DEBUG logs on DD_TRACE_DEBUG=true\n")
		tracerConfigDatabricks.TraceDebug = config.BoolToPtr(true)
	}
	if os.Getenv("DD_PROFILING_ENABLED") == "true" {
		s.Out.WriteString("Enabling Datadog Profiler on DD_PROFILING_ENABLED=true\n")
		profilingEnabled := "true"
		tracerConfigDatabricks.ProfilingEnabled = &profilingEnabled
	}
	s.Config.ApplicationMonitoringYAML = &config.ApplicationMonitoringConfig{
		Default: tracerConfigDatabricks,
	}

	// Disable credit card obfuscation for Databricks to avoid issues with job and run ids
	s.Config.DatadogYAML.APMConfig.ObfuscationConfig = &config.ObfuscationConfig{
		CreditCards: config.CreditCardObfuscationConfig{
			KeepValues: []string{"databricks_job_id", "databricks_job_run_id", "databricks_task_run_id", "config.spark_app_startTime", "config.spark_databricks_job_parentRunId"},
		},
	}

	setupCommonHostTags(s)
	installMethod := "manual"
	if os.Getenv("DD_DJM_INIT_IS_MANAGED_INSTALL") == "true" {
		installMethod = "managed"
	}
	s.Span.SetTag("install_method", installMethod)

	if os.Getenv("DD_GPU_ENABLED") == "true" {
		setupGPUIntegration(s)
	}

	switch os.Getenv("DB_IS_DRIVER") {
	case "TRUE":
		setupDatabricksDriver(s)
	default:
		setupDatabricksWorker(s)
	}
	if s.Config.DatadogYAML.LogsEnabled != nil && *s.Config.DatadogYAML.LogsEnabled {
		loadLogProcessingRules(s)
	}
	return nil
}

// normalizeWorkspaceName normalizes a workspace name to match Python's normalize_tags behavior:
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

func prefixWithWorkspace(normalizedWorkspace, value string) string {
	if normalizedWorkspace != "" {
		return normalizedWorkspace + "-" + value
	}
	return value
}

func setupCommonHostTags(s *common.Setup) {
	setIfExists(s, "DB_DRIVER_IP", "spark_host_ip", nil)
	setIfExists(s, "DB_INSTANCE_TYPE", "databricks_instance_type", nil)
	setClearIfExists(s, "DB_IS_JOB_CLUSTER", "databricks_is_job_cluster", nil)
	setClearIfExists(s, "DATABRICKS_RUNTIME_VERSION", "databricks_runtime", nil)
	setClearIfExists(s, "SPARK_SCALA_VERSION", "scala_version", nil)
	var sanitizedJobName string
	if jobName, ok := os.LookupEnv("DD_JOB_NAME"); ok {
		sanitizedJobName = jobNameRegex.ReplaceAllString(jobName, "_")
		setHostTag(s, "job_name", sanitizedJobName)
	}

	// duplicated for backward compatibility
	setIfExists(s, "DB_CLUSTER_NAME", "databricks_cluster_name", func(v string) string {
		return clusterNameRegex.ReplaceAllString(v, "_")
	})
	setIfExists(s, "DB_CLUSTER_ID", "databricks_cluster_id", nil)

	// for legacy reasons, we keep databricks_workspace un-normalized and normalized in workspace
	setIfExists(s, "DATABRICKS_WORKSPACE", "databricks_workspace", nil)
	var normalizedWorkspace string
	if workspace, ok := os.LookupEnv("DATABRICKS_WORKSPACE"); ok {
		normalizedWorkspace = normalizeWorkspaceName(workspace)
		setClearHostTag(s, "workspace", normalizedWorkspace)
	}
	setIfExists(s, "WORKSPACE_URL", "workspace_url", nil)

	setClearIfExists(s, "DB_CLUSTER_ID", "cluster_id", nil)
	setIfExists(s, "DB_CLUSTER_NAME", "cluster_name", func(v string) string {
		return clusterNameRegex.ReplaceAllString(v, "_")
	})

	jobID, runID, ok := getJobAndRunIDs()
	if ok {
		setHostTag(s, "jobid", jobID)
		setHostTag(s, "runid", runID)
		if sanitizedJobName != "" {
			setHostTag(s, "dd.internal.resource:databricks_job", prefixWithWorkspace(normalizedWorkspace, sanitizedJobName))
		} else {
			setHostTag(s, "dd.internal.resource:databricks_job", prefixWithWorkspace(normalizedWorkspace, jobID))
		}
	}
	setHostTag(s, "data_workload_monitoring_trial", "true")

	// Set databricks_cluster resource tag based on whether we're on a job cluster
	isJobCluster, _ := os.LookupEnv("DB_IS_JOB_CLUSTER")
	if isJobCluster == "TRUE" && ok {
		if sanitizedJobName != "" {
			setHostTag(s, "dd.internal.resource:databricks_cluster", prefixWithWorkspace(normalizedWorkspace, sanitizedJobName))
		} else {
			setHostTag(s, "dd.internal.resource:databricks_cluster", prefixWithWorkspace(normalizedWorkspace, jobID))
		}
	} else {
		if clusterID, ok := os.LookupEnv("DB_CLUSTER_ID"); ok {
			setHostTag(s, "dd.internal.resource:databricks_cluster", prefixWithWorkspace(normalizedWorkspace, clusterID))
		}
	}

	addCustomHostTags(s)
}

func getJobAndRunIDs() (jobID, runID string, ok bool) {
	clusterName := os.Getenv("DB_CLUSTER_NAME")
	if !strings.HasPrefix(clusterName, "job-") {
		return "", "", false
	}
	if !strings.Contains(clusterName, "-run-") {
		return "", "", false
	}
	parts := strings.Split(clusterName, "-")
	if len(parts) < 4 {
		return "", "", false
	}
	if parts[0] != "job" || parts[2] != "run" {
		return "", "", false
	}
	return parts[1], parts[3], true
}

func setIfExists(s *common.Setup, envKey, tagKey string, normalize func(string) string) {
	value, ok := os.LookupEnv(envKey)
	if !ok {
		return
	}
	if normalize != nil {
		value = normalize(value)
	}
	setHostTag(s, tagKey, value)
}

func setClearIfExists(s *common.Setup, envKey, tagKey string, normalize func(string) string) {
	value, ok := os.LookupEnv(envKey)
	if !ok {
		return
	}
	if normalize != nil {
		value = normalize(value)
	}
	setClearHostTag(s, tagKey, value)
}

func setHostTag(s *common.Setup, tagKey, value string) {
	s.Config.DatadogYAML.Tags = append(s.Config.DatadogYAML.Tags, tagKey+":"+value)
	isTagPresent := "false"
	if value != "" {
		isTagPresent = "true"
	}
	s.Span.SetTag("host_tag_set."+tagKey, isTagPresent)
}

func setClearHostTag(s *common.Setup, tagKey, value string) {
	s.Config.DatadogYAML.Tags = append(s.Config.DatadogYAML.Tags, tagKey+":"+value)
	s.Span.SetTag("host_tag_value."+tagKey, value)
}

// setupGPUIntegration configures GPU monitoring integration
func setupGPUIntegration(s *common.Setup) {
	s.Out.WriteString("Setting up GPU monitoring based on env variable GPU_MONITORING_ENABLED=true\n")
	s.Span.SetTag("host_tag_set.gpu_monitoring_enabled", "true")

	s.Config.DatadogYAML.GPUCheck.Enabled = config.BoolToPtr(true)

	// Agent must be restarted after NVML initialization, which occurs after init script execution
	s.DelayedAgentRestartConfig.Scheduled = true
	s.DelayedAgentRestartConfig.Delay = gpuIntegrationRestartDelay
	s.DelayedAgentRestartConfig.LogFile = restartLogFile
}

func setupPrivilegedLogs(s *common.Setup, sparkNode string) {
	s.Out.WriteString("Setting up privileged logs with system probe for standard access mode\n")
	s.Span.SetTag("host_tag_set.privileged_logs_enabled", "true")

	var originalLogPath string
	if sparkNode == "driver" {
		originalLogPath = "/databricks/driver/logs"
	} else if sparkNode == "worker" {
		originalLogPath = "/databricks/spark/work"
	}
	if s.Config.SystemProbeYAML == nil {
		s.Config.SystemProbeYAML = &config.SystemProbeConfig{}
	}
	s.Config.SystemProbeYAML.PrivilegedLogsConfig.Enabled = config.BoolToPtr(true)

	if err := os.MkdirAll("/var/log/databricks_privileged", 0755); err != nil {
		log.Warnf("Failed to create /var/log/databricks_privileged directory: %v", err)
		return
	}
	if err := os.MkdirAll(originalLogPath, 0755); err != nil {
		log.Warnf("Failed to create %s directory: %v", originalLogPath, err)
		return
	}
	_, err := common.ExecuteCommandWithTimeout(s, "mount", "--bind", originalLogPath, "/var/log/databricks_privileged")
	if err != nil {
		log.Warnf("Failed to mount driver logs: %v", err)
	}
}

func setupDatabricksDriver(s *common.Setup) {
	s.Out.WriteString("Setting up Spark integration config on the Driver\n")
	setClearHostTag(s, "spark_node", "driver")

	var sparkIntegration config.IntegrationConfig
	if os.Getenv("DRIVER_LOGS_ENABLED") == "true" {
		s.Config.DatadogYAML.LogsEnabled = config.BoolToPtr(true)

		// Use mounted disk log collection by default, unless explicitly disabled
		if os.Getenv("STANDARD_ACCESS_MODE") == "false" {
			sparkIntegration.Logs = driverLogs
			s.Span.SetTag("host_tag_set.driver_logs_enabled", "true")
		} else {
			setupPrivilegedLogs(s, "driver")
			sparkIntegration.Logs = driverLogsStandardAccessMode
			s.Span.SetTag("host_tag_set.driver_logs_enabled_standard_am", "true")
		}
	}
	if os.Getenv("DB_DRIVER_IP") != "" {
		sparkIntegration.Instances = []any{
			config.IntegrationConfigInstanceSpark{
				SparkURL:         "http://" + os.Getenv("DB_DRIVER_IP") + ":40001",
				SparkClusterMode: "spark_driver_mode",
				ClusterName:      os.Getenv("DB_CLUSTER_NAME"),
				StreamingMetrics: true,
			},
		}
	} else {
		log.Warn("DB_DRIVER_IP not set")
	}
	s.Config.IntegrationConfigs["spark.d/databricks.yaml"] = sparkIntegration
}

func setupDatabricksWorker(s *common.Setup) {
	setClearHostTag(s, "spark_node", "worker")
	var sparkIntegration config.IntegrationConfig

	if os.Getenv("WORKER_LOGS_ENABLED") == "true" {
		s.Config.DatadogYAML.LogsEnabled = config.BoolToPtr(true)

		// Use mounted disk log collection by default, unless explicitly disabled
		if os.Getenv("STANDARD_ACCESS_MODE") == "false" {
			sparkIntegration.Logs = workerLogs
			s.Span.SetTag("host_tag_set.worker_logs_enabled", "true")
		} else {
			setupPrivilegedLogs(s, "worker")
			sparkIntegration.Logs = workerLogsStandardAccessMode
			s.Span.SetTag("host_tag_set.worker_logs_enabled_standard_am", "true")
		}
	}
	if os.Getenv("DB_DRIVER_IP") != "" && os.Getenv("DD_EXECUTORS_SPARK_INTEGRATION") == "true" {
		sparkIntegration.Instances = []any{
			config.IntegrationConfigInstanceSpark{
				SparkURL:         "http://" + os.Getenv("DB_DRIVER_IP") + ":40001",
				SparkClusterMode: "spark_driver_mode",
				ClusterName:      os.Getenv("DB_CLUSTER_NAME"),
				StreamingMetrics: true,
			},
		}
		s.Span.SetTag("host_tag_set.worker_spark_enabled", "true")
	}
	s.Config.IntegrationConfigs["spark.d/databricks.yaml"] = sparkIntegration
}

func addCustomHostTags(s *common.Setup) {
	tags := os.Getenv("DD_TAGS")
	extraTags := os.Getenv("DD_EXTRA_TAGS")

	// Split by comma or space because agent uses space and old script uses comma
	tagsArray := strings.FieldsFunc(tags, func(r rune) bool {
		return r == ',' || r == ' '
	})
	extraTagsArray := strings.FieldsFunc(extraTags, func(r rune) bool {
		return r == ',' || r == ' '
	})

	for _, tag := range tagsArray {
		if tag != "" {
			s.Config.DatadogYAML.Tags = append(s.Config.DatadogYAML.Tags, tag)
		}
	}
	for _, tag := range extraTagsArray {
		if tag != "" {
			s.Config.DatadogYAML.ExtraTags = append(s.Config.DatadogYAML.ExtraTags, tag)
		}
	}
	s.Span.SetTag("host_tag_set.dd_tags", len(tagsArray))
	s.Span.SetTag("host_tag_set.dd_extra_tags", len(extraTagsArray))
}

func parseLogProcessingRules(input string) ([]config.LogProcessingRule, error) {
	var rules []config.LogProcessingRule
	// single quote are invalid for string in json
	jsonInput := strings.ReplaceAll(input, `'`, `"`)
	err := json.Unmarshal([]byte(jsonInput), &rules)
	if err != nil {
		return nil, err
	}
	return rules, nil
}

func loadLogProcessingRules(s *common.Setup) {
	if rawRules := os.Getenv("DD_LOGS_CONFIG_PROCESSING_RULES"); rawRules != "" {
		processingRules, err := parseLogProcessingRules(rawRules)
		if err != nil {
			log.Warnf("Failed to parse log processing rules: %v", err)
			s.Out.WriteString(fmt.Sprintf("Invalid log processing rules: %v\n", err))
		} else {
			logsConfig := config.LogsConfig{ProcessingRules: processingRules}
			s.Config.DatadogYAML.LogsConfig = logsConfig
			s.Out.WriteString(fmt.Sprintf("Loaded %d log processing rule(s) from DD_LOGS_CONFIG_PROCESSING_RULES\n", len(processingRules)))
			s.Span.SetTag("host_tag_set.log_rules", len(processingRules))
		}
	}
}
