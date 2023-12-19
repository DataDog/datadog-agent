// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudproviders

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/gce"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	networkIDCacheKey = "networkID"
)

// GetNetworkID retrieves the network_id which can be used to improve network
// connection resolution. This can be configured or detected.  The
// following sources will be queried:
// * configuration
// * GCE
// * EC2
func GetNetworkID(ctx context.Context) (string, error) {
	return cache.Get[string](
		networkIDCacheKey,
		func() (string, error) {
			// the the id from configuration
			if networkID := config.Datadog.GetString("network.id"); networkID != "" {
				log.Debugf("GetNetworkID: using configured network ID: %s", networkID)
				return networkID, nil
			}

			log.Debugf("GetNetworkID trying GCE")
			if networkID, err := gce.GetNetworkID(ctx); err == nil {
				log.Debugf("GetNetworkID: using network ID from GCE metadata: %s", networkID)
				return networkID, nil
			}

			log.Debugf("GetNetworkID trying EC2")
			if networkID, err := ec2.GetNetworkID(ctx); err == nil {
				log.Debugf("GetNetworkID: using network ID from EC2 metadata: %s", networkID)
				return networkID, nil
			}

			return "", fmt.Errorf("could not detect network ID")
		})
}
