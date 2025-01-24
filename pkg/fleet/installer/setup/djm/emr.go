// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package djm contains data-jobs-monitoring installation logic
package djm

import (
	"encoding/json"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/setup/common"
	"github.com/DataDog/datadog-agent/pkg/fleet/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"os"
	"path/filepath"
	"strconv"
)

const (
	emrInjectorVersion   = "0.26.0-1"
	emrJavaTracerVersion = "1.42.2-1"
	emrAgentVersion      = "7.58.2-1"
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
	logProcessor = []common.IntegrationConfigLogProcessor{
		{
			Type:   "regex_parser",
			Source: "filename",
			Regex:  "/var/log/hadoop-yarn/containers/(?P<application_id>application_[0-9]+_[0-9]+)/(?P<container_id>container_[0-9]+_[0-9]+_[0-9]+_[0-9]+)",
			Target: "attributes",
		},
	}
	emrLogs = []common.IntegrationConfigLogs{
		{
			Type:       "file",
			Path:       hadoopLogFolder + "*/*/stdout",
			Source:     "worker_logs",
			Service:    "hadoop-yarn",
			Tags:       "log_source:stdout",
			Processors: logProcessor,
		},
		{
			Type:       "file",
			Path:       hadoopLogFolder + "*/*/stderr",
			Source:     "worker_logs",
			Service:    "hadoop-yarn",
			Tags:       "log_source:stderr",
			Processors: logProcessor,
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

	s.Packages.Install(common.DatadogAgentPackage, emrAgentVersion)
	s.Packages.Install(common.DatadogAPMInjectPackage, emrInjectorVersion)
	s.Packages.Install(common.DatadogAPMLibraryJavaPackage, emrJavaTracerVersion)

	os.Setenv("DD_APM_INSTRUMENTATION_ENABLED", "host")

	hostname, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("failed to get hostname: %w", err)
	}
	s.Config.DatadogYAML.Hostname = hostname
	s.Config.DatadogYAML.DJM.Enabled = true
	s.Config.InjectTracerYAML.AdditionalEnvironmentVariables = tracerEnvConfigEmr

	// Ensure tags are always attached with the metrics
	s.Config.DatadogYAML.ExpectedTagsDuration = "10m"
	isMaster, clusterName, err := setupCommonEmrHostTags(s)
	if err != nil {
		return fmt.Errorf("failed to set tags: %w", err)
	}
	if isMaster {
		setupResourceManager(s, clusterName)
	}
	// Add logs config to both Resource Manager and Workers
	var sparkIntegration common.IntegrationConfig
	if os.Getenv("DD_EMR_LOGS_ENABLED") == "true" {
		s.Config.DatadogYAML.LogsEnabled = true
		sparkIntegration.Logs = emrLogs
	}
	s.Config.IntegrationConfigs["spark.d/conf.yaml"] = sparkIntegration
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
	setHostTag(s, "is_master_node", strconv.FormatBool(info.IsMaster))
	s.Span.SetTag("host."+"is_master_node", info.IsMaster)

	extraInstanceInfoRaw, err := os.ReadFile(filepath.Join(emrInfoPath, "extraInstanceData.json"))
	if err != nil {
		return info.IsMaster, "", fmt.Errorf("error reading extra instance data file: %w", err)
	}

	var extraInfo extraEmrInstanceInfo
	if err = json.Unmarshal(extraInstanceInfoRaw, &extraInfo); err != nil {
		return info.IsMaster, "", fmt.Errorf("error umarshalling extra instance data file: %w", err)
	}
	setHostTag(s, "job_flow_id", extraInfo.JobFlowID)
	setHostTag(s, "cluster_id", extraInfo.JobFlowID)
	setHostTag(s, "emr_version", extraInfo.ReleaseLabel)
	s.Span.SetTag("emr_version", extraInfo.ReleaseLabel)
	setHostTag(s, "data_workload_monitoring_trial", "true")

	clusterName := resolveEmrClusterName(s, extraInfo.JobFlowID)
	setHostTag(s, "cluster_name", clusterName)

	return info.IsMaster, clusterName, nil
}

func setupResourceManager(s *common.Setup, clusterName string) {

	var sparkIntegration common.IntegrationConfig
	var yarnIntegration common.IntegrationConfig

	sparkIntegration.Instances = []any{
		common.IntegrationConfigInstanceSpark{
			SparkURL:         "http://127.0.0.1:8088",
			SparkClusterMode: "spark_yarn_mode",
			ClusterName:      clusterName,
			StreamingMetrics: false,
		},
	}
	yarnIntegration.Instances = []any{
		common.IntegrationConfigInstanceYarn{
			ResourceManagerURI: "http://127.0.0.1:8088",
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
