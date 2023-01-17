// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/logs"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	logshttp "github.com/DataDog/datadog-agent/pkg/logs/client/http"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func NewLogContextCompliance() (*config.Endpoints, *client.DestinationsContext, error) {
	logsConfigComplianceKeys := config.NewLogsConfigKeys("compliance_config.endpoints.", coreconfig.Datadog)
	return NewLogContext(logsConfigComplianceKeys, "cspm-intake.", "compliance", config.DefaultIntakeOrigin, logs.AgentJSONIntakeProtocol)
}

func NewLogContext(logsConfig *config.LogsConfigKeys, endpointPrefix string, intakeTrackType config.IntakeTrackType, intakeOrigin config.IntakeOrigin, intakeProtocol config.IntakeProtocol) (*config.Endpoints, *client.DestinationsContext, error) {
	endpoints, err := config.BuildHTTPEndpointsWithConfig(logsConfig, endpointPrefix, intakeTrackType, intakeProtocol, intakeOrigin)
	if err != nil {
		endpoints, err = config.BuildHTTPEndpoints(intakeTrackType, intakeProtocol, intakeOrigin)
		if err == nil {
			httpConnectivity := logshttp.CheckConnectivity(endpoints.Main)
			endpoints, err = config.BuildEndpoints(httpConnectivity, intakeTrackType, intakeProtocol, intakeOrigin)
		}
	}

	if err != nil {
		return nil, nil, log.Errorf("Invalid endpoints: %v", err)
	}

	for _, status := range endpoints.GetStatus() {
		log.Info(status)
	}

	destinationsCtx := client.NewDestinationsContext()
	destinationsCtx.Start()

	return endpoints, destinationsCtx, nil
}
