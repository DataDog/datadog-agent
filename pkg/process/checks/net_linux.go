// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package checks

import (
	"context"
	"fmt"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/process/net"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/network"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/port/portlist"
)

// getListeningPortToPIDMap returns a map of listening port -> PID using the portlist Poller
func getListeningPortToPIDMap() map[int32]int32 {
	poller := &portlist.Poller{IncludeLocalhost: true}
	defer poller.Close()

	ports, _, err := poller.Poll()
	if err != nil {
		log.Debugf("failed to poll listening ports: %v", err)
		return nil
	}
	result := make(map[int32]int32, len(ports))
	for _, p := range ports {
		if p.Pid > 0 {
			result[int32(p.Port)] = int32(p.Pid)
		}
	}
	return result
}

// fetchIISTagsCache is not applicable on Linux; returns nil.
func fetchIISTagsCache(_ *http.Client) map[string][]string {
	return nil
}

// getNetworkID fetches network_id from the current netNS or from the system probe if necessary, where the root netNS is used
func getNetworkID(sysProbeClient *http.Client) (string, error) {
	networkID, err := network.GetNetworkID(context.Background())
	if err != nil {
		if sysProbeClient == nil {
			return "", fmt.Errorf("no network ID detected and system-probe client not available: %w", err)
		}
		log.Debugf("no network ID detected. retrying via system-probe: %s", err)
		networkID, err = net.GetNetworkID(sysProbeClient)
		if err != nil {
			log.Debugf("failed to get network ID from system-probe: %s", err)
			return "", fmt.Errorf("failed to get network ID from system-probe: %w", err)
		}
	}
	return networkID, err
}
