// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package djm contains data-jobs-monitoring installation logic
package djm

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/setup/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	databricksInjectorVersion   = "0.35.0-1"
	databricksJavaTracerVersion = "1.46.1-1"
	databricksAgentVersion      = "7.63.3-1"
)

var (
	jobNameRegex     = regexp.MustCompile(`[,\']`)
	clusterNameRegex = regexp.MustCompile(`[^a-zA-Z0-9_:.-]`)
	driverLogs       = []common.IntegrationConfigLogs{
		{
			Type:                   "file",
			Path:                   "/databricks/driver/logs/*.log",
			Source:                 "driver_logs",
			Service:                "databricks",
			AutoMultiLineDetection: true,
		},
		{
			Type:                   "file",
			Path:                   "/databricks/driver/logs/stderr",
			Source:                 "driver_stderr",
			Service:                "databricks",
			AutoMultiLineDetection: true,
		},
		{
			Type:                   "file",
			Path:                   "/databricks/driver/logs/stdout",
			Source:                 "driver_stdout",
			Service:                "databricks",
			AutoMultiLineDetection: true,
		},
	}
	workerLogs = []common.IntegrationConfigLogs{
		{
			Type:                   "file",
			Path:                   "/databricks/spark/work/*/*/*.log",
			Source:                 "worker_logs",
			Service:                "databricks",
			AutoMultiLineDetection: true,
		},
		{
			Type:                   "file",
			Path:                   "/databricks/spark/work/*/*/stderr",
			Source:                 "worker_stderr",
			Service:                "databricks",
			AutoMultiLineDetection: true,
		},
		{
			Type:                   "file",
			Path:                   "/databricks/spark/work/*/*/stdout",
			Source:                 "worker_stdout",
			Service:                "databricks",
			AutoMultiLineDetection: true,
		},
	}
	tracerEnvConfigDatabricks = []common.InjectTracerConfigEnvVar{
		{
			Key:   "DD_DATA_JOBS_ENABLED",
			Value: "true",
		},
		{
			Key:   "DD_INTEGRATIONS_ENABLED",
			Value: "false",
		},
	}
)

// SetupDatabricks sets up the Databricks environment
func SetupDatabricks(s *common.Setup) error {
	s.Packages.InstallInstaller()
	s.Packages.Install(common.DatadogAgentPackage, databricksAgentVersion)
	s.Packages.Install(common.DatadogAPMInjectPackage, databricksInjectorVersion)
	s.Packages.Install(common.DatadogAPMLibraryJavaPackage, databricksJavaTracerVersion)

	s.Out.WriteString("Applying specific Data Jobs Monitoring config\n")
	hostname, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("failed to get hostname: %w", err)
	}
	s.Config.DatadogYAML.Hostname = hostname
	s.Config.DatadogYAML.DJM.Enabled = true
	s.Config.DatadogYAML.ExpectedTagsDuration = "10m"
	s.Config.DatadogYAML.ProcessConfig.ExpvarPort = 6063 // avoid port conflict on 6062

	if os.Getenv("DD_TRACE_DEBUG") == "true" {
		s.Out.WriteString("Enabling Datadog Java Tracer DEBUG logs on DD_TRACE_DEBUG=true\n")
		debugLogs := common.InjectTracerConfigEnvVar{
			Key:   "DD_TRACE_DEBUG",
			Value: "true",
		}
		tracerEnvConfigEmr = append(tracerEnvConfigDatabricks, debugLogs)
	}
	s.Config.InjectTracerYAML.AdditionalEnvironmentVariables = tracerEnvConfigDatabricks

	setupCommonHostTags(s)
	installMethod := "manual"
	if os.Getenv("DD_DJM_INIT_IS_MANAGED_INSTALL") == "true" {
		installMethod = "managed"
	}
	s.Span.SetTag("install_method", installMethod)

	switch os.Getenv("DB_IS_DRIVER") {
	case "TRUE":
		setupDatabricksDriver(s)
	default:
		setupDatabricksWorker(s)
	}
	return nil
}

func setupCommonHostTags(s *common.Setup) {
	setIfExists(s, "DB_DRIVER_IP", "spark_host_ip", nil)
	setIfExists(s, "DB_INSTANCE_TYPE", "databricks_instance_type", nil)
	setClearIfExists(s, "DB_IS_JOB_CLUSTER", "databricks_is_job_cluster", nil)
	setIfExists(s, "DD_JOB_NAME", "job_name", func(v string) string {
		return jobNameRegex.ReplaceAllString(v, "_")
	})

	// duplicated for backward compatibility
	setIfExists(s, "DB_CLUSTER_NAME", "databricks_cluster_name", func(v string) string {
		return clusterNameRegex.ReplaceAllString(v, "_")
	})
	setIfExists(s, "DB_CLUSTER_ID", "databricks_cluster_id", nil)

	setIfExists(s, "DATABRICKS_WORKSPACE", "databricks_workspace", nil)
	setClearIfExists(s, "DATABRICKS_WORKSPACE", "workspace", func(v string) string {
		return strings.Trim(v, "\"'")
	})

	setClearIfExists(s, "DB_CLUSTER_ID", "cluster_id", nil)
	setIfExists(s, "DB_CLUSTER_NAME", "cluster_name", func(v string) string {
		return clusterNameRegex.ReplaceAllString(v, "_")
	})

	jobID, runID, ok := getJobAndRunIDs()
	if ok {
		setHostTag(s, "jobid", jobID)
		setHostTag(s, "runid", runID)
	}
	setHostTag(s, "data_workload_monitoring_trial", "true")
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
	s.Span.SetTag("host_tag_set."+tagKey, "true")
}

func setClearHostTag(s *common.Setup, tagKey, value string) {
	s.Config.DatadogYAML.Tags = append(s.Config.DatadogYAML.Tags, tagKey+":"+value)
	s.Span.SetTag("host_tag_value."+tagKey, value)
}

func setupDatabricksDriver(s *common.Setup) {
	s.Out.WriteString("Setting up Spark integration config on the Driver\n")
	setClearHostTag(s, "spark_node", "driver")

	var sparkIntegration common.IntegrationConfig
	if os.Getenv("DRIVER_LOGS_ENABLED") == "true" {
		s.Config.DatadogYAML.LogsEnabled = true
		sparkIntegration.Logs = driverLogs
	}
	if os.Getenv("DB_DRIVER_IP") != "" {
		sparkIntegration.Instances = []any{
			common.IntegrationConfigInstanceSpark{
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

	var sparkIntegration common.IntegrationConfig
	if os.Getenv("WORKER_LOGS_ENABLED") == "true" {
		s.Config.DatadogYAML.LogsEnabled = true
		sparkIntegration.Logs = workerLogs
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
