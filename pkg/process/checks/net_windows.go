// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package checks

import (
	"context"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/network"
)

// getNetworkID fetches network_id
func getNetworkID(_ *http.Client) (string, error) {
	return network.GetNetworkID(context.Background())
}
