// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package command

import (
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/logs"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	logshttp "github.com/DataDog/datadog-agent/pkg/logs/client/http"
	logsconfig "github.com/DataDog/datadog-agent/pkg/logs/config"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	cwsIntakeOrigin logsconfig.IntakeOrigin = "cloud-workload-security"
)

func NewLogContextCompliance() (*logsconfig.Endpoints, *client.DestinationsContext, error) {
	logsConfigComplianceKeys := logsconfig.NewLogsConfigKeys("compliance_config.endpoints.", pkgconfig.Datadog)
	return NewLogContext(logsConfigComplianceKeys, "cspm-intake.", "compliance", logsconfig.DefaultIntakeOrigin, logs.AgentJSONIntakeProtocol)
}

// This function will only be used on Linux. The only platforms where the runtime agent runs
func NewLogContextRuntime() (*logsconfig.Endpoints, *client.DestinationsContext, error) {
	logsConfigComplianceKeys := logsconfig.NewLogsConfigKeys("runtime_security_config.endpoints.", pkgconfig.Datadog)
	return NewLogContext(logsConfigComplianceKeys, "runtime-security-http-intake.logs.", "logs", cwsIntakeOrigin, logsconfig.DefaultIntakeProtocol)
}

func NewLogContext(logsConfig *logsconfig.LogsConfigKeys, endpointPrefix string, intakeTrackType logsconfig.IntakeTrackType, intakeOrigin logsconfig.IntakeOrigin, intakeProtocol logsconfig.IntakeProtocol) (*logsconfig.Endpoints, *client.DestinationsContext, error) {
	endpoints, err := logsconfig.BuildHTTPEndpointsWithConfig(logsConfig, endpointPrefix, intakeTrackType, intakeProtocol, intakeOrigin)
	if err != nil {
		endpoints, err = logsconfig.BuildHTTPEndpoints(intakeTrackType, intakeProtocol, intakeOrigin)
		if err == nil {
			httpConnectivity := logshttp.CheckConnectivity(endpoints.Main)
			endpoints, err = logsconfig.BuildEndpoints(httpConnectivity, intakeTrackType, intakeProtocol, intakeOrigin)
		}
	}

	if err != nil {
		return nil, nil, pkglog.Errorf("Invalid endpoints: %v", err)
	}

	for _, status := range endpoints.GetStatus() {
		pkglog.Info(status)
	}

	destinationsCtx := client.NewDestinationsContext()
	destinationsCtx.Start()

	return endpoints, destinationsCtx, nil
}
