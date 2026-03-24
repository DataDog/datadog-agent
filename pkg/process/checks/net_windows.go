// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package checks

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/network"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// defaultFetchTimeout is the context timeout for HTTP requests to system-probe endpoints.
const defaultFetchTimeout = 10 * time.Second

// getNetworkID fetches network_id
func getNetworkID(_ *http.Client) (string, error) {
	return network.GetNetworkID(context.Background())
}

// fetchFromSystemProbe fetches JSON from a system-probe module endpoint and decodes into T.
// Returns the zero value of T on any failure.
func fetchFromSystemProbe[T any](client *http.Client, path string) T {
	var zero T
	ctx, cancel := context.WithTimeout(context.Background(), defaultFetchTimeout)
	defer cancel()
	url := sysprobeclient.ModuleURL(sysconfig.NetworkTracerModule, path)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		log.Debugf("failed to create request for %s: %v", path, err)
		return zero
	}
	resp, err := client.Do(req)
	if err != nil {
		log.Debugf("failed to fetch %s from system-probe: %v", path, err)
		return zero
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Debugf("%s request failed with status %d", path, resp.StatusCode)
		return zero
	}
	var result T
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Debugf("failed to decode %s response: %v", path, err)
		return zero
	}
	return result
}

// fetchIISTagsCache retrieves the IIS tags cache from system-probe's /iis_tags endpoint.
func fetchIISTagsCache(client *http.Client) map[string][]string {
	return fetchFromSystemProbe[map[string][]string](client, "/iis_tags")
}

// fetchProcessCacheTags retrieves process cache tags from system-probe's /process_cache_tags endpoint.
func fetchProcessCacheTags(client *http.Client) map[uint32][]string {
	return fetchFromSystemProbe[map[uint32][]string](client, "/process_cache_tags")
}

// fetchRemoteServiceData fetches IIS tags, process cache tags, and the listening
// port-to-PID map concurrently, as each involves an I/O operation.
func fetchRemoteServiceData(client *http.Client) (map[string][]string, map[uint32][]string, map[int32]int32) {
	var iisTags map[string][]string
	var procCacheTags map[uint32][]string
	var portToPID map[int32]int32
	var wg sync.WaitGroup
	wg.Add(3)
	go func() { defer wg.Done(); iisTags = fetchIISTagsCache(client) }()
	go func() { defer wg.Done(); procCacheTags = fetchProcessCacheTags(client) }()
	go func() { defer wg.Done(); portToPID = getListeningPortToPIDMap() }()
	wg.Wait()
	return iisTags, procCacheTags, portToPID
}

// getRemoteProcessTags returns process tags for a remote PID using the system-probe process cache.
func getRemoteProcessTags(pid int32, procCacheTags map[uint32][]string, _ func(int32) ([]string, error)) []string {
	if procCacheTags == nil {
		return nil
	}
	return procCacheTags[uint32(pid)]
}
