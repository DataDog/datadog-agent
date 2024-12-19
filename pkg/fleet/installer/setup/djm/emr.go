// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package djm contains data-jobs-monitoring installation logic
package djm

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/setup/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const (
	emrInjectorVersion     = "0.26.0-1"
	emrJavaVersion         = "1.42.2-1"
	emrAgentVersion        = "7.58.2-1"
	commandTimeoutDuration = 10 * time.Second
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
	IsMaster        string `json:"isMaster"`
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
	s.Packages.Install(common.DatadogAPMLibraryJavaPackage, emrJavaVersion)

	hostname, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("failed to get hostname: %w", err)
	}
	s.Config.DatadogYAML.Hostname = hostname
	s.Config.DatadogYAML.DJM.Enabled = true

	s.Config.DatadogYAML.ExpectedTagsDuration = "10m"
	isMaster, clusterName, err := setupCommonEmrHostTags(s)
	if err != nil {
		return fmt.Errorf("failed to set tags: %w", err)
	}
	if isMaster == "true" {
		setupEmrDriver(s, hostname, clusterName)
	}
	return nil
}

func setupCommonEmrHostTags(s *common.Setup) (string, string, error) {

	instanceInfoRaw, err := os.ReadFile(filepath.Join(emrInfoPath, "instance.json"))
	if err != nil {
		return "", "", fmt.Errorf("error reading instance file: %w", err)
	}

	var info emrInstanceInfo
	if err = json.Unmarshal(instanceInfoRaw, &info); err != nil {
		return "", "", fmt.Errorf("error umarshalling instance file: %w", err)
	}

	setHostTag(s, "instance_group_id", info.InstanceGroupID)
	setHostTag(s, "is_master_node", info.IsMaster)
	s.Span.SetTag("host_tag."+"is_master_node", info.IsMaster)

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
	s.Span.SetTag("emr_version", extraInfo.ReleaseLabel)

	emrResponseRaw, err := executeCommandWithTimeout("aws", "emr", "describe-cluster", "--cluster-id", extraInfo.JobFlowID)
	if err != nil {
		return "", "", err
	}
	var response emrResponse
	if err = json.Unmarshal(emrResponseRaw, &response); err != nil {
		return info.IsMaster, "", fmt.Errorf("error unmarshalling AWS EMR response: %w", err)
	}

	setHostTag(s, "cluster_name", response.Cluster.Name)
	return info.IsMaster, response.Cluster.Name, nil
}

func setupEmrDriver(s *common.Setup, host string, clusterName string) {

	s.Config.InjectTracerYAML.AdditionalEnvironmentVariables = tracerEnvConfigEmr

	var sparkIntegration common.IntegrationConfig
	var yarnIntegration common.IntegrationConfig

	if host != "" {
		sparkIntegration.Instances = []any{
			common.IntegrationConfigInstanceSpark{
				SparkURL:         "http://" + host + ":8088",
				SparkClusterMode: "spark_yarn_mode",
				ClusterName:      clusterName,
				StreamingMetrics: false,
			},
		}
		yarnIntegration.Instances = []any{
			common.IntegrationConfigInstanceYarn{
				ResourceManagerURI: "http://" + host + ":8088",
				ClusterName:        clusterName,
			},
		}
		s.Config.IntegrationConfigs["spark.d/conf.yaml"] = sparkIntegration
		s.Config.IntegrationConfigs["yarn.d/conf.yaml"] = yarnIntegration
	} else {
		log.Warn("host is empty, Spark and yarn integrations are not set up")
	}
}

var executeCommandWithTimeout = func(command string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeoutDuration)
	defer cancel()

	cmd := exec.CommandContext(ctx, command, args...)
	output, err := cmd.Output()

	if err != nil {
		return []byte(""), fmt.Errorf("error executing command: %w", err)
	}

	return output, nil
}
