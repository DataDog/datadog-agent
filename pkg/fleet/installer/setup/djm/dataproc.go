// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package djm contains data-jobs-monitoring installation logic
package djm

import (
	"cloud.google.com/go/compute/metadata"
	"context"
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/setup/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	dataprocInjectorVersion   = "0.26.0-1"
	dataprocJavaTracerVersion = "1.42.2-1"
	dataprocAgentVersion      = "7.58.2-1"
)

var (
	tracerEnvConfigDataproc = []common.InjectTracerConfigEnvVar{
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
	}
)

// SetupDataproc sets up the DJM environment on Dataproc
func SetupDataproc(s *common.Setup) error {

	metadataClient := metadata.NewClient(nil)
	s.Packages.Install(common.DatadogAgentPackage, dataprocAgentVersion)
	s.Packages.Install(common.DatadogAPMInjectPackage, dataprocInjectorVersion)
	s.Packages.Install(common.DatadogAPMLibraryJavaPackage, dataprocJavaTracerVersion)

	os.Setenv("DD_APM_INSTRUMENTATION_ENABLED", "host")

	hostname, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("failed to get hostname: %w", err)
	}
	s.Config.DatadogYAML.Hostname = hostname
	s.Config.DatadogYAML.DJM.Enabled = true
	s.Config.InjectTracerYAML.AdditionalEnvironmentVariables = tracerEnvConfigDataproc

	// Ensure tags are always attached with the metrics
	s.Config.DatadogYAML.ExpectedTagsDuration = "10m"
	isMaster, clusterName, err := setupCommonDataprocHostTags(s, metadataClient)
	if err != nil {
		return fmt.Errorf("failed to set tags: %w", err)
	}
	if isMaster == "true" {
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
	setHostTag(s, "cluster_id", clusterID)
	setHostTag(s, "dataproc_cluster_id", clusterID)

	dataprocRole, err := metadataClient.InstanceAttributeValueWithContext(ctx, "dataproc-role")
	if err != nil {
		return "", "", err
	}
	isMaster := "false"
	if dataprocRole == "Master" {
		isMaster = "true"
	}
	setHostTag(s, "is_master_node", isMaster)
	s.Span.SetTag("host."+"is_master_node", isMaster)

	clusterName, err := metadataClient.InstanceAttributeValueWithContext(ctx, "dataproc-cluster-name")
	if err != nil {
		log.Warn("failed to get clusterName, using clusterID instead")
		return isMaster, clusterID, nil
	}
	setHostTag(s, "cluster_name", clusterName)

	return isMaster, clusterName, nil
}
