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
)

// InstrumentationCheckClient is the interface used by the instrumentation
// checks config provider to retrieve AD configurations from the cluster agent.
type InstrumentationCheckClient interface {
	GetInstrumentationConfigs(ctx context.Context) (types.ConfigResponse, error)
}

// GetInstrumentationConfigs is called by the instrumentation checks config provider
// to retrieve AD configurations derived from DatadogInstrumentation CRs.
func (c *DCAClient) GetInstrumentationConfigs(ctx context.Context) (types.ConfigResponse, error) {
	var configs types.ConfigResponse
	err := c.doJSONQuery(ctx, dcaInstrumentationConfigsPath, "GET", nil, &configs, false)
	return configs, err
}
