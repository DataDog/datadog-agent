// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusteragent

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
)

const (
	dcaInstrumentationConfigsPath = "api/v1/instrumentation/configs"
	dcaInstrumentationStatusPath  = "api/v1/instrumentation/status"
)

// InstrumentationCheckClient is the interface used by the instrumentation
// checks config provider to retrieve AD configurations from the cluster agent.
type InstrumentationCheckClient interface {
	GetInstrumentationConfigs(ctx context.Context) (types.InstrumentationConfigResponse, error)
	GetInstrumentationStatus(ctx context.Context) (types.InstrumentationStatusResponse, error)
}

// GetInstrumentationConfigs is called by the instrumentation checks config provider
// to retrieve AD configurations derived from DatadogInstrumentation CRs.
func (c *DCAClient) GetInstrumentationConfigs(ctx context.Context) (types.InstrumentationConfigResponse, error) {
	var configs types.InstrumentationConfigResponse
	err := c.doJSONQuery(ctx, dcaInstrumentationConfigsPath, "GET", nil, &configs, false)
	return configs, err
}

// GetInstrumentationStatus returns the hash of the latest instrumentation
// config state. Node agents use this to avoid fetching the full config payload
// when nothing has changed.
func (c *DCAClient) GetInstrumentationStatus(ctx context.Context) (types.InstrumentationStatusResponse, error) {
	var status types.InstrumentationStatusResponse
	err := c.doJSONQuery(ctx, dcaInstrumentationStatusPath, "GET", nil, &status, false)
	return status, err
}
