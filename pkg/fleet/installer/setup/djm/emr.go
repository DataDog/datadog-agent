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
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	emrInjectorVersion   = "0.35.0-1"
	emrJavaTracerVersion = "1.48.0-1"
	emrAgentVersion      = "7.63.3-1"
	hadoopLogFolder      = "/var/log/hadoop-yarn/containers/"
)

var (
	emrInfoPath        = "/mnt/var/lib/info"
	tracerEnvConfigEmr = []common.InjectTracerConfigEnvVar{
		{
			Key:   "DD_DATA_JOBS_ENABLED",
			Value: "true",
		},
		{
			Key:   "DD_INTEGRATIONS_ENABLED",
			Value: "false",
		},
		{
			Key:   "DD_DATA_JOBS_COMMAND_PATTERN",
			Value: ".*org.apache.spark.deploy.*",
		},
		{
			Key:   "DD_SPARK_APP_NAME_AS_SERVICE",
			Value: "true",
		},
		{
			Key:   "DD_INJECT_FORCE",
			Value: "true",
		},
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
	s.Packages.InstallInstaller()
	s.Packages.Install(common.DatadogAgentPackage, emrAgentVersion)
	s.Packages.Install(common.DatadogAPMInjectPackage, emrInjectorVersion)
	s.Packages.Install(common.DatadogAPMLibraryJavaPackage, emrJavaTracerVersion)

	s.Out.WriteString("Applying specific Data Jobs Monitoring config\n")
	os.Setenv("DD_APM_INSTRUMENTATION_ENABLED", "host")

	hostname, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("failed to get hostname: %w", err)
	}
	s.Config.DatadogYAML.Hostname = hostname
	s.Config.DatadogYAML.DJM.Enabled = true

	if os.Getenv("DD_DATA_STREAMS_ENABLED") == "true" {
		s.Out.WriteString("Propagating variable DD_DATA_STREAMS_ENABLED=true to tracer configuration\n")
		DSMEnabled := common.InjectTracerConfigEnvVar{
			Key:   "DD_DATA_STREAMS_ENABLED",
			Value: "true",
		}
		tracerEnvConfigEmr = append(tracerEnvConfigEmr, DSMEnabled)
	}
	if os.Getenv("DD_TRACE_DEBUG") == "true" {
		s.Out.WriteString("Enabling Datadog Java Tracer DEBUG logs on DD_TRACE_DEBUG=true\n")
		debugLogs := common.InjectTracerConfigEnvVar{
			Key:   "DD_TRACE_DEBUG",
			Value: "true",
		}
		tracerEnvConfigEmr = append(tracerEnvConfigEmr, debugLogs)
	}
	s.Config.InjectTracerYAML.AdditionalEnvironmentVariables = tracerEnvConfigEmr
	// Ensure tags are always attached with the metrics
	s.Config.DatadogYAML.ExpectedTagsDuration = "10m"
	isMaster, clusterName, err := setupCommonEmrHostTags(s)
	if err != nil {
		return fmt.Errorf("failed to set tags: %w", err)
	}
	if isMaster {
		s.Out.WriteString("Setting up Spark integration config on the Resource Manager\n")
		setupResourceManager(s, clusterName)
	}
	// Add logs config to both Resource Manager and Workers
	if os.Getenv("DD_EMR_LOGS_ENABLED") == "true" {
		s.Out.WriteString("Enabling EMR logs collection based on env variable DD_EMR_LOGS_ENABLED=true\n")
		enableEmrLogs(s)
	} else {
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
	var sparkIntegration common.IntegrationConfig
	var yarnIntegration common.IntegrationConfig
	hostname, err := os.Hostname()
	if err != nil {
		log.Infof("Failed to get hostname, defaulting to localhost: %v", err)
		hostname = "localhost"
	}
	sparkIntegration.Instances = []any{
		common.IntegrationConfigInstanceSpark{
			SparkURL:         "http://" + hostname + ":8088",
			SparkClusterMode: "spark_yarn_mode",
			ClusterName:      clusterName,
			StreamingMetrics: false,
		},
	}
	yarnIntegration.Instances = []any{
		common.IntegrationConfigInstanceYarn{
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

func enableEmrLogs(s *common.Setup) {
	s.Config.DatadogYAML.LogsEnabled = true
	s.Span.SetTag("host_tag_set.logs_enabled", "true")
	// Add dd-agent user to yarn group so that it gets read permission to the hadoop-yarn logs folder
	s.DdAgentAdditionalGroups = append(s.DdAgentAdditionalGroups, "yarn")
	// Load the existing integration config and add logs section to it
	sparkIntegration := s.Config.IntegrationConfigs["spark.d/conf.yaml"]
	emrLogs := []common.IntegrationConfigLogs{
		{
			Type:    "file",
			Path:    hadoopLogFolder + "*/*/stdout",
			Source:  "hadoop-yarn",
			Service: "emr-logs",
			LogProcessingRules: []common.LogProcessingRule{
				{Type: "multi_line", Name: "dataframe_show", Pattern: "`\\|[\\sa-zA-Z-_.\\|]+\\|$`gm"},
			},
		},
		{
			Type:    "file",
			Path:    hadoopLogFolder + "*/*/stderr",
			Source:  "hadoop-yarn",
			Service: "emr-logs",
		},
	}
	sparkIntegration.Logs = emrLogs
	s.Config.IntegrationConfigs["spark.d/conf.yaml"] = sparkIntegration
}
