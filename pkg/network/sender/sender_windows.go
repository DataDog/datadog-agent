// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows && npm

package sender

import (
	httpprotocol "github.com/DataDog/datadog-agent/pkg/network/protocols/http"
)

// fetchIISTagsCache retrieves the IIS tags cache
func fetchIISTagsCache() map[string][]string {
	return httpprotocol.GetIISTagsCache()
}

// fetchProcessCacheTags retrieves process cache tags
func fetchProcessCacheTags(tracer ConnectionsSource) map[uint32][]string {
	return tracer.GetProcessCacheTags()
}

// fetchServiceData fetches IIS tags, process cache tags, and the listeners
func fetchServiceData(tracer ConnectionsSource) (map[string][]string, map[uint32][]string, map[listenKey]int32) {
	return fetchIISTagsCache(), fetchProcessCacheTags(tracer), getListeningPortToPIDMap()
}

// getProcessTags returns process tags for a PID using the system-probe process cache.
func getProcessTags(pid int32, procCacheTags map[uint32][]string, _ func(int32) ([]string, error)) []string {
	if procCacheTags == nil {
		return nil
	}
	return procCacheTags[uint32(pid)]
}
