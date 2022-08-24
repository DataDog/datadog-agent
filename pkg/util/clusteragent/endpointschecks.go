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
	dcaEndpointsChecksPath        = "api/v1/endpointschecks"
	dcaEndpointsChecksConfigsPath = dcaEndpointsChecksPath + "/configs"
)

// GetEndpointsCheckConfigs is called by the endpointscheck config provider
func (c *DCAClient) GetEndpointsCheckConfigs(ctx context.Context, nodeName string) (types.ConfigResponse, error) {
	var configs types.ConfigResponse

	// https://host:port/api/v1/endpointschecks/configs/{nodeName}
	err := c.doJSONQueryToLeader(ctx, dcaEndpointsChecksConfigsPath+"/"+nodeName, "GET", nil, &configs)
	return configs, err
}
