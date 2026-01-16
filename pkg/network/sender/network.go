// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package sender

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/network"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	netnsutil "github.com/DataDog/datadog-agent/pkg/util/kernel/netns"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// retryGetNetworkID attempts to fetch the network_id maxRetries times before failing
// as the endpoint is sometimes unavailable during host startup
func retryGetNetworkID(ctx context.Context) (string, error) {
	const maxRetries = 4
	var err error
	var networkID string
	for attempt := 1; attempt <= maxRetries; attempt++ {
		networkID, err = getNetworkID(ctx)
		if err == nil {
			return networkID, nil
		}
		log.Debugf(
			"failed to fetch network ID (attempt %d/%d): %s",
			attempt,
			maxRetries,
			err,
		)
		if attempt < maxRetries {
			time.Sleep(time.Duration(250*attempt) * time.Millisecond)
		}
	}
	return "", fmt.Errorf("failed to get network ID after %d attempts: %w", maxRetries, err)
}

func getNetworkID(ctx context.Context) (id string, err error) {
	err = netnsutil.WithRootNS(kernel.ProcFSRoot(), func() error {
		id, err = network.GetNetworkID(ctx)
		return err
	})
	return
}
