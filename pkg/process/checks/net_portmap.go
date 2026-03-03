// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux || windows

package checks

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/port/portlist"
)

var (
	portPollerOnce sync.Once
	portPoller     *portlist.Poller
	cachedPortMap  map[int32]int32
)

// getListeningPortToPIDMap returns a map of listening port -> PID using a
// long-lived portlist.Poller. The Poller is created once and reused across
// calls so that it can benefit from scratch-buffer reuse. When the port list
// has not changed since the last poll the cached map is returned.
func getListeningPortToPIDMap() map[int32]int32 {
	portPollerOnce.Do(func() {
		portPoller = &portlist.Poller{IncludeLocalhost: true}
	})

	ports, changed, err := portPoller.Poll()
	if err != nil {
		log.Debugf("failed to poll listening ports: %v", err)
		return cachedPortMap // return stale data if available
	}
	if !changed && cachedPortMap != nil {
		return cachedPortMap
	}

	result := make(map[int32]int32, len(ports))
	for _, p := range ports {
		if p.Pid > 0 {
			result[int32(p.Port)] = int32(p.Pid)
		}
	}
	cachedPortMap = result
	return result
}
