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
)

// getNetworkID fetches network_id
func getNetworkID(_ *http.Client) (string, error) {
	return network.GetNetworkID(context.Background())
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

// fetchProcessCacheTags retrieves process cache tags from system-probe's /process_cache_tags endpoint.
// Returns a map of PID (as uint32) -> []string tags, or nil on failure.
func fetchProcessCacheTags(client *http.Client) map[uint32][]string {
	url := sysprobeclient.ModuleURL(sysconfig.NetworkTracerModule, "/process_cache_tags")
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Debugf("failed to create process cache tags request: %v", err)
		return nil
	}
	resp, err := client.Do(req)
	if err != nil {
		log.Debugf("failed to fetch process cache tags from system-probe: %v", err)
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Debugf("process cache tags request failed with status %d", resp.StatusCode)
		return nil
	}
	var result map[uint32][]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Debugf("failed to decode process cache tags response: %v", err)
		return nil
	}
	return result
}

// getRemoteProcessTags returns process tags for a remote PID using the system-probe process cache.
func getRemoteProcessTags(pid int32, procCacheTags map[uint32][]string, _ func(int32) ([]string, error)) []string {
	if procCacheTags == nil {
		return nil
	}
	return procCacheTags[uint32(pid)]
}
