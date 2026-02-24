// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package checks

import (
	"context"
	"encoding/json"
	"net/http"

	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/network"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/port/portlist"
)

// getNetworkID fetches network_id
func getNetworkID(_ *http.Client) (string, error) {
	return network.GetNetworkID(context.Background())
}

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

// fetchIISTagsCache retrieves the IIS tags cache from system-probe's /iis_tags endpoint.
// Returns a map of "localPort-remotePort" -> []string tags, or nil on failure.
func fetchIISTagsCache(client *http.Client) map[string][]string {
	url := sysprobeclient.ModuleURL(sysconfig.NetworkTracerModule, "/iis_tags")
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Debugf("failed to create IIS tags request: %v", err)
		return nil
	}
	resp, err := client.Do(req)
	if err != nil {
		log.Debugf("failed to fetch IIS tags from system-probe: %v", err)
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Debugf("IIS tags request failed with status %d", resp.StatusCode)
		return nil
	}
	var result map[string][]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Debugf("failed to decode IIS tags response: %v", err)
		return nil
	}
	return result
}
