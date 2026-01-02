// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package common holds common related files
package common

import (
	"fmt"

	logsconfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// TrackType represents the type of track for event routing
type TrackType = logsconfig.IntakeTrackType

const (
	cwsIntakeOrigin logsconfig.IntakeOrigin = "cloud-workload-security"

	// SecRuntime is the track type for secruntime events
	SecRuntime TrackType = "secruntime"
	// Logs is the track type for logs events
	Logs TrackType = "logs"
	// SecInfo is the track type for secinfo events
	SecInfo TrackType = "secinfo"
)

// NewLogContextCompliance returns the context fields to send compliance events to the intake
func NewLogContextCompliance() (*logsconfig.Endpoints, *client.DestinationsContext, error) {
	logsConfigComplianceKeys := logsconfig.NewLogsConfigKeys("compliance_config.endpoints.", pkgconfigsetup.Datadog())
	return NewLogContext(logsConfigComplianceKeys, "cspm-intake.", "compliance", logsconfig.DefaultIntakeOrigin, logsconfig.AgentJSONIntakeProtocol)
}

// NewLogContextRuntime returns the context fields to send runtime (CWS) events to the intake
// This function will only be used on Linux. The only platforms where the runtime agent runs
func NewLogContextRuntime(useSecRuntimeTrack bool) (*logsconfig.Endpoints, *client.DestinationsContext, error) {
	var trackType TrackType

	if useSecRuntimeTrack {
		trackType = SecRuntime
	} else {
		trackType = Logs
	}

	logsRuntimeConfigKeys := logsconfig.NewLogsConfigKeys("runtime_security_config.endpoints.", pkgconfigsetup.Datadog())
	return NewLogContext(logsRuntimeConfigKeys, "runtime-security-http-intake.logs.", trackType, cwsIntakeOrigin, logsconfig.DefaultIntakeProtocol)
}

// NewLogContextSecInfo returns the context fields to send remediation events to the intake
func NewLogContextSecInfo() (*logsconfig.Endpoints, *client.DestinationsContext, error) {
	logsRuntimeConfigKeys := logsconfig.NewLogsConfigKeys("runtime_security_config.endpoints.", pkgconfigsetup.Datadog())
	return NewLogContext(logsRuntimeConfigKeys, "runtime-security-http-intake.logs.", SecInfo, cwsIntakeOrigin, logsconfig.DefaultIntakeProtocol)
}

// NewLogContext returns the context fields to send events to the intake
func NewLogContext(logsConfig *logsconfig.LogsConfigKeys, endpointPrefix string, intakeTrackType logsconfig.IntakeTrackType, intakeOrigin logsconfig.IntakeOrigin, intakeProtocol logsconfig.IntakeProtocol) (*logsconfig.Endpoints, *client.DestinationsContext, error) {
	endpoints, err := logsconfig.BuildHTTPEndpointsWithConfig(pkgconfigsetup.Datadog(), logsConfig, endpointPrefix, intakeTrackType, intakeProtocol, intakeOrigin)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid endpoints: %w", err)
	}

	for _, status := range endpoints.GetStatus() {
		log.Info(status)
	}

	destinationsCtx := client.NewDestinationsContext()
	destinationsCtx.Start()

	return endpoints, destinationsCtx, nil
}
