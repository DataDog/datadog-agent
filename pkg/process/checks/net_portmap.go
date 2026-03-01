// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux || windows

package checks

import (
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
