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
	"strings"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/setup/common"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/setup/config"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	emrInjectorVersion    = "0.67.2-1"
	emrJavaTracerVersion  = "1.63.0-1"
	emrAgentVersion       = "7.79.2-1"
	emrOpenLineageVersion = "1.49.0"
	hadoopDriverFolder    = "/mnt/var/log/hadoop/steps/"
)

var (
	emrInfoPath       = "/mnt/var/lib/info"
	openLineageJARDir = "/usr/lib/spark/jars"
	tracerConfigEmr   = config.APMConfigurationDefault{
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
		tracerConfigEmr.TraceDebug = config.BoolToPtr(true)
	}
	setupOpenLineage(s)
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
		// Add logs config to the Resource Manager only
		if os.Getenv("DD_EMR_LOGS_ENABLED") == "true" {
			s.Out.WriteString("Enabling EMR logs collection\n")
			s.Span.SetTag("host_tag_set.logs_enabled", "true")
			enableEmrLogs(s)
		} else if os.Getenv("DD_EMR_DRIVER_LOGS_ENABLED") == "true" {
			// Tag install spans to track usage of DD_EMR_DRIVER_LOGS_ENABLED versus DD_EMR_LOGS_ENABLED
			s.Span.SetTag("host_tag_set.driver_logs_enabled", "true")
			enableEmrLogs(s)
		} else if os.Getenv("DD_EMR_DRIVER_LOGS_ENABLED") != "true" {
			s.Out.WriteString("EMR logs collection not enabled. To enable it, set DD_EMR_LOGS_ENABLED=true\n")
		}
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
		setEmrClusterNameSpanTags(span, "job_flow_id", "AWS EMR describe-cluster failed; using job flow ID as cluster name", err.Error())
		err = nil
		return jobFlowID
	}
	var response emrResponse
	if err = json.Unmarshal(emrResponseRaw, &response); err != nil {
		log.Warnf("error unmarshalling AWS EMR response,  using cluster id as name: %v", err)
		setEmrClusterNameSpanTags(span, "job_flow_id", "Could not parse AWS EMR describe-cluster response; using job flow ID as cluster name", err.Error())
		err = nil
		return jobFlowID
	}
	clusterName := response.Cluster.Name
	if clusterName == "" {
		log.Warn("clusterName is empty, using cluster id as name")
		setEmrClusterNameSpanTags(span, "job_flow_id", "AWS EMR describe-cluster returned an empty cluster name; using job flow ID as cluster name", "")
		return jobFlowID
	}
	setEmrClusterNameSpanTags(span, "aws_emr_describe_cluster", "Resolved cluster name from AWS EMR describe-cluster", "")
	return clusterName
}

func setEmrClusterNameSpanTags(span *telemetry.Span, source, reason, errorMessage string) {
	span.SetTag("cluster_name_source", source)
	span.SetTag("cluster_name_resolution_reason", reason)
	if errorMessage != "" {
		span.SetTag("cluster_name_resolution_error_message", errorMessage)
	}
}

func setupOpenLineage(s *common.Setup) {
	if os.Getenv("DD_OPENLINEAGE_ENABLED") != "true" {
		return
	}
	s.Out.WriteString("Enabling OpenLineage integration\n")
	tracerConfigEmr.DataJobsOpenLineageEnabled = config.BoolToPtr(true)
	s.Span.SetTag("host_tag_set.openlineage_enabled", "true")

	// Check if an OpenLineage JAR is already on the classpath
	matches, err := filepath.Glob(filepath.Join(openLineageJARDir, "openlineage-spark*.jar"))
	if err == nil && len(matches) > 0 {
		s.Out.WriteString(fmt.Sprintf("OpenLineage JAR already present (%s), skipping download\n", matches[0]))
		return
	}

	variant := detectScalaVariant(s)
	jarName := fmt.Sprintf("openlineage-spark%s-%s.jar", variant, emrOpenLineageVersion)
	jarDest := filepath.Join(openLineageJARDir, jarName)

	// Allow pre-staged JAR via DD_OPENLINEAGE_JAR_PATH for VPCs without internet
	if jarPath := os.Getenv("DD_OPENLINEAGE_JAR_PATH"); jarPath != "" {
		s.Out.WriteString(fmt.Sprintf("Copying OpenLineage JAR from %s\n", jarPath))
		if _, err := common.ExecuteCommandWithTimeout(s, "cp", jarPath, jarDest); err != nil {
			log.Warnf("failed to copy OpenLineage JAR from %s: %v", jarPath, err)
		}
		return
	}

	jarURL := fmt.Sprintf(
		"https://repo1.maven.org/maven2/io/openlineage/openlineage-spark%s/%s/%s",
		variant, emrOpenLineageVersion, jarName,
	)
	s.Out.WriteString(fmt.Sprintf("Downloading OpenLineage JAR v%s\n", emrOpenLineageVersion))
	if _, err := common.ExecuteCommandWithTimeout(s, "curl", "-sSfL", "-o", jarDest, jarURL); err != nil {
		log.Warnf("failed to download OpenLineage JAR: %v", err)
	}
}

// detectScalaVariant returns the Scala binary version suffix (e.g. "_2.12") to use when
// downloading the OpenLineage JAR. It checks, in order:
//  1. DD_OPENLINEAGE_SPARK_VARIANT env var — for clusters where auto-detection is unavailable
//  2. scala-library-*.jar in Spark's jars directory — reliable on most EMR releases
//  3. Falls back to "_2.12" with a warning (covers EMR 6.x/7.x; EMR 8.x uses 2.13)
func detectScalaVariant(s *common.Setup) string {
	if v := os.Getenv("DD_OPENLINEAGE_SPARK_VARIANT"); v != "" {
		return v
	}
	matches, _ := filepath.Glob(filepath.Join(openLineageJARDir, "scala-library-*.jar"))
	if len(matches) == 0 {
		log.Warn("Could not detect Scala variant from Spark jars directory — no scala-library-*.jar found. Falling back to Scala 2.12 for OpenLineage JAR download; set DD_OPENLINEAGE_SPARK_VARIANT to override.")
		s.Out.WriteString("WARNING: Could not detect Scala variant, falling back to Scala 2.12 for OpenLineage JAR\n")
		return "_2.12"
	}
	// e.g. "scala-library-2.12.18.jar" → "_2.12"
	base := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(matches[0]), "scala-library-"), ".jar")
	parts := strings.SplitN(base, ".", 3)
	if len(parts) < 2 {
		log.Warnf("Could not parse Scala version from JAR name %q. Falling back to Scala 2.12 for OpenLineage JAR download; set DD_OPENLINEAGE_SPARK_VARIANT to override.", filepath.Base(matches[0]))
		s.Out.WriteString("WARNING: Could not parse Scala variant from JAR name, falling back to Scala 2.12 for OpenLineage JAR\n")
		return "_2.12"
	}
	return "_" + parts[0] + "." + parts[1]
}

func enableEmrLogs(s *common.Setup) {
	s.Config.DatadogYAML.LogsEnabled = config.BoolToPtr(true)
	loadLogProcessingRules(s)
	// Load the existing integration config and add logs section to it
	sparkIntegration := s.Config.IntegrationConfigs["spark.d/conf.yaml"]
	emrLogs := []config.IntegrationConfigLogs{
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
	sparkIntegration.Logs = emrLogs
	s.Config.IntegrationConfigs["spark.d/conf.yaml"] = sparkIntegration
}
