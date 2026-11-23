// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package gpu contains GPU Monitoring installation logic
package gpu

import (
	"fmt"
	"os"
	"regexp"
	"time"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/setup/common"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/setup/config"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/setup/djm"
)

const (
	// defaultAgentVersion is a major version tag, so every install run pulls
	// the latest 7.x Agent. Customers can pin it with DD_AGENT_MINOR_VERSION.
	defaultAgentVersion       = "7"
	databricksInjectorVersion = "0.71.2-1"
	// defaultCTracerVersion is a major version tag, so every install run pulls
	// the latest 0.x release of dd-trace-c.
	defaultCTracerVersion = "0"
	// envCTracerVersion lets customers pin the C tracer version, e.g. 0.40.0
	envCTracerVersion = "DD_C_TRACER_VERSION"

	gpuRestartDelay   = 60 * time.Second
	gpuRestartLogFile = "/var/log/datadog-gpu-restart"
)

// fullVersionWithoutRevision matches versions like 0.40.0, which are
// published with a package revision suffix (0.40.0-1).
var fullVersionWithoutRevision = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

// SetupDatabricksGPU sets up GPU Monitoring on Databricks: GPU metrics and
// GPU traces from the C tracer (dd-trace-c), which the APM injector loads into
// processes. It does not set up Data Jobs Monitoring.
func SetupDatabricksGPU(s *common.Setup) error {
	if os.Getenv("DD_NO_AGENT_INSTALL") != "true" {
		agentVersion := agentVersion(s)
		s.Out.WriteString(fmt.Sprintf("Installing the Datadog Agent version %s\n", agentVersion))
		s.Packages.Install(common.DatadogAgentPackage, agentVersion)
	}
	s.Packages.Install(common.DatadogAPMInjectPackage, databricksInjectorVersion)
	version := cTracerVersion()
	s.Out.WriteString(fmt.Sprintf("Installing the Datadog C tracer version %s\n", version))
	s.Packages.Install(common.DatadogAPMLibraryCPackage, version)
	s.Span.SetTag("c_tracer_version", version)

	hostname, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("failed to get hostname: %w", err)
	}
	s.Config.DatadogYAML.Hostname = hostname
	s.Config.DatadogYAML.ExpectedTagsDuration = "10m"
	s.Config.DatadogYAML.ProcessConfig.ExpvarPort = 6063 // avoid port conflict on 6062

	djm.SetupDatabricksHostTags(s)
	setupGPUCheck(s)
	return nil
}

// setupGPUCheck enables the GPU check and schedules an Agent restart, since
// NVML is initialized after the Databricks init script runs.
func setupGPUCheck(s *common.Setup) {
	s.Out.WriteString("Setting up GPU monitoring\n")
	s.Config.DatadogYAML.GPUCheck.Enabled = config.BoolToPtr(true)
	s.DelayedAgentRestartConfig.Scheduled = true
	s.DelayedAgentRestartConfig.Delay = gpuRestartDelay
	s.DelayedAgentRestartConfig.LogFile = gpuRestartLogFile
}

// agentVersion returns the Agent version requested with DD_AGENT_MAJOR_VERSION
// and DD_AGENT_MINOR_VERSION (e.g. DD_AGENT_MINOR_VERSION=86.0), or the latest
// Agent 7 when none is requested.
func agentVersion(s *common.Setup) string {
	if s.Env == nil {
		return defaultAgentVersion
	}
	version := s.Env.GetAgentVersion()
	if version == "latest" {
		return defaultAgentVersion
	}
	return version
}

func cTracerVersion() string {
	version := os.Getenv(envCTracerVersion)
	if version == "" {
		return defaultCTracerVersion
	}
	if fullVersionWithoutRevision.MatchString(version) {
		return version + "-1"
	}
	return version
}
