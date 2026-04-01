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
)

// fetchRemoteServiceData returns remote service enrichment data. On Linux,
// IIS tags and process cache tags are not applicable; only portToPID is fetched.
func fetchRemoteServiceData(_ *http.Client) (map[string][]string, map[uint32][]string, map[int32]int32) {
	return nil, nil, getListeningPortToPIDMap()
}

// getRemoteProcessTags returns process tags for a remote PID using the tagger.
func getRemoteProcessTags(pid int32, _ map[uint32][]string, processTagProvider func(int32) ([]string, error)) []string {
	if processTagProvider == nil {
		return nil
	}
	tags, err := processTagProvider(pid)
	if err != nil {
		log.Debugf("error getting process tags for remote pid %d: %v", pid, err)
		return nil
	}
	return tags
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
