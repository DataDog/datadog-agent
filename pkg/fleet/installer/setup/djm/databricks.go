// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package djm contains data-jobs-monitoring installation logic
package djm

import (
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/setup/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	databricksInjectorVersion = "0.21.0"
	databricksJavaVersion     = "1.41.1"
	databricksAgentVersion    = "7.57.2"
)

var (
	envToTags = map[string]string{
		"DATABRICKS_WORKSPACE": "workspace",
		"DB_CLUSTER_NAME":      "databricks_cluster_name",
		"DB_CLUSTER_ID":        "databricks_cluster_id",
		"DB_NODE_TYPE":         "databricks_node_type",
	}
	driverLogs = []common.IntegrationConfigLogs{
		{
			Type:    "file",
			Path:    "/databricks/driver/logs/*.log",
			Source:  "driver_logs",
			Service: "databricks",
		},
		{
			Type:    "file",
			Path:    "/databricks/driver/logs/stderr",
			Source:  "driver_stderr",
			Service: "databricks",
		},
		{
			Type:    "file",
			Path:    "/databricks/driver/logs/stdout",
			Source:  "driver_stdout",
			Service: "databricks",
		},
	}
	workerLogs = []common.IntegrationConfigLogs{
		{
			Type:    "file",
			Path:    "/databricks/spark/work/*/*/*.log",
			Source:  "worker_logs",
			Service: "databricks",
		},
		{
			Type:    "file",
			Path:    "/databricks/spark/work/*/*/stderr",
			Source:  "worker_stderr",
			Service: "databricks",
		},
		{
			Type:    "file",
			Path:    "/databricks/spark/work/*/*/stdout",
			Source:  "worker_stdout",
			Service: "databricks",
		},
	}
	tracerEnvConfig = []common.InjectTracerConfigEnvVar{
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
	s.Packages.Install(common.DatadogAgentPackage, databricksAgentVersion)

	hostname, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("failed to get hostname: %w", err)
	}
	s.Config.DatadogYAML.Hostname = hostname
	s.Config.DatadogYAML.DJM.Enabled = true
	s.Config.DatadogYAML.ExpectedTagsDuration = "10m"
	s.Config.DatadogYAML.ProcessConfig.ExpvarPort = -1 // avoid port conflict
	for env, tag := range envToTags {
		if val, ok := os.LookupEnv(env); ok {
			s.Config.DatadogYAML.Tags = append(s.Config.DatadogYAML.Tags, tag+":"+val)
		}
	}

	switch os.Getenv("DB_IS_DRIVER") {
	case "true":
		setupDatabricksDriver(s)
	default:
		setupDatabricksWorker(s)
	}
	return nil
}

func setupDatabricksDriver(s *common.Setup) {
	s.Span.SetTag("spark_node", "driver")

	s.Packages.Install(common.DatadogAPMInjectPackage, databricksInjectorVersion)
	s.Packages.Install(common.DatadogAPMLibraryJavaPackage, databricksJavaVersion)

	s.Config.DatadogYAML.Tags = append(s.Config.DatadogYAML.Tags, "node_type:driver")
	s.Config.InjectTracerYAML.EnvsToInject = tracerEnvConfig

	var sparkIntegration common.IntegrationConfig
	if os.Getenv("DRIVER_LOGS_ENABLED") == "true" {
		sparkIntegration.Logs = driverLogs
	}
	if os.Getenv("DB_DRIVER_IP") != "" || os.Getenv("DB_DRIVER_PORT") != "" {
		sparkIntegration.Instances = []any{
			common.IntegrationConfigInstanceSpark{
				SparkURL:         "http://" + os.Getenv("DB_DRIVER_IP") + ":" + os.Getenv("DB_DRIVER_PORT"),
				SparkClusterMode: "spark_driver_mode",
				ClusterName:      os.Getenv("DB_CLUSTER_NAME"),
				StreamingMetrics: true,
			},
		}
	} else {
		log.Warn("DB_DRIVER_IP or DB_DRIVER_PORT not set")
	}
	s.Config.IntegrationConfigs["spark.d/databricks.yaml"] = sparkIntegration
}

func setupDatabricksWorker(s *common.Setup) {
	s.Span.SetTag("spark_node", "worker")

	s.Packages.Install(common.DatadogAgentPackage, databricksAgentVersion)

	s.Config.DatadogYAML.Tags = append(s.Config.DatadogYAML.Tags, "node_type:worker")

	var sparkIntegration common.IntegrationConfig
	if os.Getenv("WORKER_LOGS_ENABLED") == "true" {
		sparkIntegration.Logs = workerLogs
	}
	s.Config.IntegrationConfigs["spark.d/databricks.yaml"] = sparkIntegration
}
