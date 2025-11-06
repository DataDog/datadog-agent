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
	"path/filepath"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/setup/common"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/setup/config"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	emrInjectorVersion   = "0.45.0-1"
	emrJavaTracerVersion = "1.55.0-1"
	emrAgentVersion      = "7.71.1-1"
	hadoopLogFolder      = "/var/log/hadoop-yarn/containers/"
	hadoopDriverFolder   = "/mnt/var/log/hadoop/steps/"
)

var (
	emrInfoPath     = "/mnt/var/lib/info"
	tracerConfigEmr = config.APMConfigurationDefault{
		DataJobsEnabled:               config.BoolToPtr(true),
		IntegrationsEnabled:           config.BoolToPtr(false),
		DataJobsCommandPattern:        ".*org.apache.spark.deploy.*",
		DataJobsSparkAppNameAsService: config.BoolToPtr(true),
	}
)

type emrInstanceInfo struct {
	IsMaster        bool   `json:"isMaster"`
	InstanceGroupID string `json:"instanceGroupId"`
}

type cluster struct {
	Name string `json:"Name"`
}

type emrResponse struct {
	Cluster cluster `json:"cluster"`
}

type extraEmrInstanceInfo struct {
	JobFlowID    string `json:"jobFlowId"`
	ReleaseLabel string `json:"releaseLabel"`
}

// SetupEmr sets up the DJM environment on EMR
func SetupEmr(s *common.Setup) error {
	if os.Getenv("DD_NO_AGENT_INSTALL") != "true" {
		s.Packages.Install(common.DatadogAgentPackage, emrAgentVersion)
	}
	s.Packages.Install(common.DatadogAPMInjectPackage, emrInjectorVersion)
	s.Packages.Install(common.DatadogAPMLibraryJavaPackage, emrJavaTracerVersion)

	s.Out.WriteString("Applying specific Data Jobs Monitoring config\n")
	os.Setenv("DD_APM_INSTRUMENTATION_ENABLED", "host")

	hostname, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("failed to get hostname: %w", err)
	}
	s.Config.DatadogYAML.Hostname = hostname
	s.Config.DatadogYAML.DJM.Enabled = config.BoolToPtr(true)

	if os.Getenv("DD_DATA_STREAMS_ENABLED") == "true" {
		s.Out.WriteString("Propagating variable DD_DATA_STREAMS_ENABLED=true to tracer configuration\n")
		tracerConfigEmr.DataStreamsEnabled = config.BoolToPtr(true)
	}
	if os.Getenv("DD_TRACE_DEBUG") == "true" {
		s.Out.WriteString("Enabling Datadog Java Tracer DEBUG logs on DD_TRACE_DEBUG=true\n")
		tracerConfigEmr.TraceDebug = config.BoolToPtr(false)
	}
	s.Config.ApplicationMonitoringYAML = &config.ApplicationMonitoringConfig{
		Default: tracerConfigEmr,
	}
	// Ensure tags are always attached with the metrics
	s.Config.DatadogYAML.ExpectedTagsDuration = "10m"
	isMaster, clusterName, err := setupCommonEmrHostTags(s)
	if err != nil {
		return fmt.Errorf("failed to set tags: %w", err)
	}
	if isMaster {
		s.Out.WriteString("Setting up Spark integration config on the Resource Manager\n")
		setupResourceManager(s, clusterName)
		if os.Getenv("DD_EMR_DRIVER_LOGS_ENABLED") == "true" {
			s.Out.WriteString("Enabling EMR logs collection from driver based on env variable DD_EMR_DRIVER_LOGS_ENABLED=true\n")
			enableEmrLogs(s, true)
		}
	}
	// Add logs config to both Resource Manager and Workers
	if os.Getenv("DD_EMR_LOGS_ENABLED") == "true" {
		s.Out.WriteString("Enabling EMR logs collection based on env variable DD_EMR_LOGS_ENABLED=true\n")
		enableEmrLogs(s, false)
	} else if os.Getenv("DD_EMR_DRIVER_LOGS_ENABLED") != "true" {
		s.Out.WriteString("EMR logs collection not enabled. To enable it, set DD_EMR_LOGS_ENABLED=true\n")
	}
	return nil
}

func setupCommonEmrHostTags(s *common.Setup) (bool, string, error) {

	instanceInfoRaw, err := os.ReadFile(filepath.Join(emrInfoPath, "instance.json"))
	if err != nil {
		return false, "", fmt.Errorf("error reading instance file: %w", err)
	}

	var info emrInstanceInfo
	if err = json.Unmarshal(instanceInfoRaw, &info); err != nil {
		return false, "", fmt.Errorf("error umarshalling instance file: %w", err)
	}

	setHostTag(s, "instance_group_id", info.InstanceGroupID)
	setClearHostTag(s, "is_master_node", strconv.FormatBool(info.IsMaster))

	extraInstanceInfoRaw, err := os.ReadFile(filepath.Join(emrInfoPath, "extraInstanceData.json"))
	if err != nil {
		return info.IsMaster, "", fmt.Errorf("error reading extra instance data file: %w", err)
	}

	var extraInfo extraEmrInstanceInfo
	if err = json.Unmarshal(extraInstanceInfoRaw, &extraInfo); err != nil {
		return info.IsMaster, "", fmt.Errorf("error umarshalling extra instance data file: %w", err)
	}
	setHostTag(s, "job_flow_id", extraInfo.JobFlowID)
	setClearHostTag(s, "cluster_id", extraInfo.JobFlowID)
	setClearHostTag(s, "emr_version", extraInfo.ReleaseLabel)
	setHostTag(s, "data_workload_monitoring_trial", "true")

	clusterName := resolveEmrClusterName(s, extraInfo.JobFlowID)
	setHostTag(s, "cluster_name", clusterName)
	addCustomHostTags(s)
	return info.IsMaster, clusterName, nil
}

func setupResourceManager(s *common.Setup, clusterName string) {
	var sparkIntegration config.IntegrationConfig
	var yarnIntegration config.IntegrationConfig
	hostname, err := os.Hostname()
	if err != nil {
		log.Infof("Failed to get hostname, defaulting to localhost: %v", err)
		hostname = "localhost"
	}
	sparkIntegration.Instances = []any{
		config.IntegrationConfigInstanceSpark{
			SparkURL:         "http://" + hostname + ":8088",
			SparkClusterMode: "spark_yarn_mode",
			ClusterName:      clusterName,
			StreamingMetrics: false,
		},
	}
	yarnIntegration.Instances = []any{
		config.IntegrationConfigInstanceYarn{
			ResourceManagerURI: "http://" + hostname + ":8088",
			ClusterName:        clusterName,
		},
	}
	s.Config.IntegrationConfigs["spark.d/conf.yaml"] = sparkIntegration
	s.Config.IntegrationConfigs["yarn.d/conf.yaml"] = yarnIntegration

}

func resolveEmrClusterName(s *common.Setup, jobFlowID string) string {
	var err error
	span, _ := telemetry.StartSpanFromContext(s.Ctx, "resolve.cluster_name")
	defer func() { span.Finish(err) }()
	emrResponseRaw, err := common.ExecuteCommandWithTimeout(s, "aws", "emr", "describe-cluster", "--cluster-id", jobFlowID)
	if err != nil {
		log.Warnf("error describing emr cluster, using cluster id as name: %v", err)
		return jobFlowID
	}
	var response emrResponse
	if err = json.Unmarshal(emrResponseRaw, &response); err != nil {
		log.Warnf("error unmarshalling AWS EMR response,  using cluster id as name: %v", err)
		return jobFlowID
	}
	clusterName := response.Cluster.Name
	if clusterName == "" {
		log.Warn("clusterName is empty, using cluster id as name")
		return jobFlowID
	}
	return clusterName
}

func enableEmrLogs(s *common.Setup, collectFromDriver bool) {
	s.Config.DatadogYAML.LogsEnabled = config.BoolToPtr(true)
	loadLogProcessingRules(s)
	// Load the existing integration config and add logs section to it
	sparkIntegration := s.Config.IntegrationConfigs["spark.d/conf.yaml"]
	var emrLogs []config.IntegrationConfigLogs
	if collectFromDriver {
		s.Span.SetTag("host_tag_set.driver_logs_enabled", "true")
		emrLogs = []config.IntegrationConfigLogs{
			{
				Type:    "file",
				Path:    hadoopDriverFolder + "*/stdout",
				Source:  "hadoop-yarn",
				Service: "emr-logs",
				LogProcessingRules: []config.LogProcessingRule{
					{Type: "multi_line", Name: "logger_dataframe_show", Pattern: "(^\\+[-+]+\\n(\\|.*\\n)+\\+[-+]+$)|^(ERROR|INFO|DEBUG|WARN|CRITICAL|NOTSET|Traceback)"},
				},
			},
			{
				Type:    "file",
				Path:    hadoopDriverFolder + "*/stderr",
				Source:  "hadoop-yarn",
				Service: "emr-logs",
			},
		}
	} else {
		s.Span.SetTag("host_tag_set.logs_enabled", "true")
		// Add dd-agent user to yarn group so that it gets read permission to the hadoop-yarn logs folder
		s.DdAgentAdditionalGroups = append(s.DdAgentAdditionalGroups, "yarn")
		emrLogs = []config.IntegrationConfigLogs{
			{
				Type:    "file",
				Path:    hadoopLogFolder + "*/*/stdout",
				Source:  "hadoop-yarn",
				Service: "emr-logs",
				LogProcessingRules: []config.LogProcessingRule{
					{Type: "multi_line", Name: "logger_dataframe_show", Pattern: "(^\\+[-+]+\\n(\\|.*\\n)+\\+[-+]+$)|^(ERROR|INFO|DEBUG|WARN|CRITICAL|NOTSET|Traceback)"},
				},
			},
			{
				Type:    "file",
				Path:    hadoopLogFolder + "*/*/stderr",
				Source:  "hadoop-yarn",
				Service: "emr-logs",
			},
		}
	}
	sparkIntegration.Logs = emrLogs
	s.Config.IntegrationConfigs["spark.d/conf.yaml"] = sparkIntegration
}
