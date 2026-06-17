// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package sender

import (
	"github.com/DataDog/datadog-agent/pkg/network/remoteservice"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// fetchServiceData fetches IIS tags, process cache tags, and the listeners
func fetchServiceData(_ ConnectionsSource) (map[string][]string, map[uint32][]string, map[remoteservice.ListenKey]int32) {
	return nil, nil, getListeningPortToPIDMap()
}

// getProcessTags returns process tags for a PID using the tagger.
func getProcessTags(pid int32, _ map[uint32][]string, processTagProvider func(int32) ([]string, error)) []string {
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
