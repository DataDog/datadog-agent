// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package djm contains data-jobs-monitoring installation logic
package djm

import (
	"context"
	"fmt"
	"os"

	"cloud.google.com/go/compute/metadata"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/setup/common"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/setup/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	dataprocInjectorVersion   = "0.45.0-1"
	dataprocJavaTracerVersion = "1.55.0-1"
	dataprocAgentVersion      = "7.71.1-1"
)

var (
	tracerConfigDataproc = config.APMConfigurationDefault{
		DataJobsEnabled:               config.BoolToPtr(true),
		IntegrationsEnabled:           config.BoolToPtr(false),
		DataJobsCommandPattern:        ".*org.apache.spark.deploy.*",
		DataJobsSparkAppNameAsService: config.BoolToPtr(true),
	}
)

// SetupDataproc sets up the DJM environment on Dataproc
func SetupDataproc(s *common.Setup) error {

	metadataClient := metadata.NewClient(nil)
	if os.Getenv("DD_NO_AGENT_INSTALL") != "true" {
		s.Packages.Install(common.DatadogAgentPackage, dataprocAgentVersion)
	}
	s.Packages.Install(common.DatadogAPMInjectPackage, dataprocInjectorVersion)
	s.Packages.Install(common.DatadogAPMLibraryJavaPackage, dataprocJavaTracerVersion)

	s.Out.WriteString("Applying specific Data Jobs Monitoring config\n")
	os.Setenv("DD_APM_INSTRUMENTATION_ENABLED", "host")

	hostname, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("failed to get hostname: %w", err)
	}
	s.Config.DatadogYAML.Hostname = hostname
	s.Config.DatadogYAML.DJM.Enabled = config.BoolToPtr(true)
	if os.Getenv("DD_TRACE_DEBUG") == "true" {
		s.Out.WriteString("Enabling Datadog Java Tracer DEBUG logs on DD_TRACE_DEBUG=true\n")
		tracerConfigDataproc.TraceDebug = config.BoolToPtr(true)
	}
	s.Config.ApplicationMonitoringYAML = &config.ApplicationMonitoringConfig{
		Default: tracerConfigDataproc,
	}

	// Ensure tags are always attached with the metrics
	s.Config.DatadogYAML.ExpectedTagsDuration = "10m"
	isMaster, clusterName, err := setupCommonDataprocHostTags(s, metadataClient)
	if err != nil {
		return fmt.Errorf("failed to set tags: %w", err)
	}
	if isMaster == "true" {
		s.Out.WriteString("Setting up Spark integration config on the Resource Manager\n")
		setupResourceManager(s, clusterName)
	}
	return nil
}

func setupCommonDataprocHostTags(s *common.Setup, metadataClient *metadata.Client) (string, string, error) {
	ctx := context.Background()

	clusterID, err := metadataClient.InstanceAttributeValueWithContext(ctx, "dataproc-cluster-uuid")
	if err != nil {
		return "", "", err
	}
	setClearHostTag(s, "cluster_id", clusterID)
	setHostTag(s, "dataproc_cluster_id", clusterID)
	setHostTag(s, "data_workload_monitoring_trial", "true")

	dataprocRole, err := metadataClient.InstanceAttributeValueWithContext(ctx, "dataproc-role")
	if err != nil {
		return "", "", err
	}
	isMaster := "false"
	if dataprocRole == "Master" {
		isMaster = "true"
	}
	setClearHostTag(s, "is_master_node", isMaster)

	clusterName, err := metadataClient.InstanceAttributeValueWithContext(ctx, "dataproc-cluster-name")
	if err != nil {
		log.Warn("failed to get clusterName, using clusterID instead")
		return isMaster, clusterID, nil
	}
	setHostTag(s, "cluster_name", clusterName)
	addCustomHostTags(s)

	return isMaster, clusterName, nil
}
