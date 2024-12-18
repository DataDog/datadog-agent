// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package djm contains data-jobs-monitoring installation logic
package djm

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/setup/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	emrInjectorVersion = "0.26.0-1"
	emrJavaVersion     = "1.42.2-1"
	emrAgentVersion    = "7.58.2-1"
)

var tracerEnvConfigEmr = []common.InjectTracerConfigEnvVar{
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
		Key:   "DD_TRACE_AGENT_URL",
		Value: "http://localhost:8126",
	},
	{
		Key:   "DD_TRACE_EXPERIMENTAL_LONG_RUNNING_ENABLED",
		Value: "true",
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

// SetupEmr sets up the DJM environment on EMR
func SetupEmr(s *common.Setup) error {
	err := getEmrInstanceData()
	if err != nil {
		return fmt.Errorf("failed to get EMR instance data: %w", err)
	}
	s.Packages.Install(common.DatadogAgentPackage, emrAgentVersion)
	s.Packages.Install(common.DatadogAPMInjectPackage, emrInjectorVersion)
	s.Packages.Install(common.DatadogAPMLibraryJavaPackage, emrJavaVersion)

	hostname, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("failed to get hostname: %w", err)
	}
	s.Config.DatadogYAML.Hostname = hostname
	s.Config.DatadogYAML.DJM.Enabled = true

	s.Config.DatadogYAML.ExpectedTagsDuration = "10m"

	setupCommonEmrHostTags(s)

	switch os.Getenv("IS_MASTER") {
	case "true":
		setupEmrDriver(s)
	default:
		setupEmrWorker(s)
	}
	return nil
}

func setupCommonEmrHostTags(s *common.Setup) {

	setIfExists(s, "CLUSTER_NAME", "cluster_name", nil)
	setIfExists(s, "CLUSTER_ID", "cluster_id", nil)
	setIfExists(s, "JOB_FLOW_ID", "job_flow_id", nil)
	setIfExists(s, "INSTANCE_GROUP_ID", "instance_group_id", nil)
}

func getEmrInstanceData() error {
	isMasterCmd := exec.Command("bash", "-c", `cat /mnt/var/lib/info/instance.json | jq -r ".isMaster"`)
	output, err := isMasterCmd.Output()
	if err != nil {
		return err
	}
	isMaster := strings.TrimSpace(string(output))
	err = os.Setenv("IS_MASTER", isMaster)
	if err != nil {
		return err
	}

	jobFlowIDCmd := exec.Command("bash", "-c", `cat /mnt/var/lib/instance-controller/extraInstanceData.json | jq -r ".jobFlowId"`)
	output, err = jobFlowIDCmd.Output()
	if err != nil {
		return err
	}
	jobFlowID := strings.TrimSpace(string(output))
	err = os.Setenv("JOB_FLOW_ID", jobFlowID)
	if err != nil {
		return err
	}

	instanceGroupIDCmd := exec.Command("bash", "-c", `cat /mnt/var/lib/info/instance.json | jq -r ".instanceGroupId"`)
	output, err = instanceGroupIDCmd.Output()
	if err != nil {
		return err
	}
	instanceGroupID := strings.TrimSpace(string(output))
	err = os.Setenv("INSTANCE_GROUP_ID", instanceGroupID)
	if err != nil {
		return err
	}

	clusterNameCmd := exec.Command("bash", "-c", `cat /mnt/var/lib/info/instance.json | jq -r ".instanceGroupId"`)
	output, err = clusterNameCmd.Output()
	if err != nil {
		return err
	}
	clusterName := strings.TrimSpace(string(output))
	err = os.Setenv("CLUSTER_NAME", clusterName)
	if err != nil {
		return err
	}
	return nil
}

func setupEmrDriver(s *common.Setup) {
	s.Span.SetTag("is_driver", "true")

	s.Packages.Install(common.DatadogAPMInjectPackage, emrInjectorVersion)
	s.Packages.Install(common.DatadogAPMLibraryJavaPackage, emrJavaVersion)

	s.Config.DatadogYAML.Tags = append(s.Config.DatadogYAML.Tags, "is_master_node:true")
	s.Config.InjectTracerYAML.AdditionalEnvironmentVariables = tracerEnvConfig

	var sparkIntegration common.IntegrationConfig
	var yarnIntegration common.IntegrationConfig

	if os.Getenv("CLUSTER_NAME") != "" {
		sparkIntegration.Instances = []any{
			common.IntegrationConfigInstanceSpark{
				SparkURL:         "http://" + os.Getenv("HOST") + ":8088",
				SparkClusterMode: "spark_yarn_mode",
				ClusterName:      os.Getenv("CLUSTER_NAME"),
				StreamingMetrics: false,
			},
		}
		yarnIntegration.Instances = []any{
			common.IntegrationConfigInstanceYarn{
				ResourceManagerURI: "http://" + os.Getenv("HOST") + ":8088",
				ClusterName:        os.Getenv("CLUSTER_NAME"),
			},
		}
	} else {
		log.Warn("CLUSTER_NAME not set")
	}
	s.Config.IntegrationConfigs["spark.d/conf.yaml"] = sparkIntegration
	s.Config.IntegrationConfigs["yarn.d/conf.yaml"] = yarnIntegration
}

func setupEmrWorker(s *common.Setup) {
	s.Span.SetTag("is_driver", "false")

	s.Config.DatadogYAML.Tags = append(s.Config.DatadogYAML.Tags, "is_master_node:false")

	var sparkIntegration common.IntegrationConfig

	s.Config.IntegrationConfigs["spark.d/databricks.yaml"] = sparkIntegration
}
