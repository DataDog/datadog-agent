// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package checks

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/process/metadata/parser"
	"github.com/DataDog/datadog-agent/pkg/process/net"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/network"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// fetchIISTagsCache is not applicable on Linux; returns nil.
func fetchIISTagsCache(_ *http.Client) map[string][]string {
	return nil
}

// fetchProcessCacheTags is not applicable on Linux; returns nil.
func fetchProcessCacheTags(_ *http.Client) map[uint32][]string {
	return nil
}

// getRemoteProcessTags returns process tags for a remote PID by reading DD_SERVICE directly from /proc/<pid>/environ.
func getRemoteProcessTags(pid int32, _ map[uint32][]string, _ func(int32) ([]string, error)) []string {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(int(pid)), "environ"))
	if err != nil {
		log.Debugf("Unable to read environ for pid %d: %v", pid, err)
		return nil
	}
	if svc, ok := parser.ChooseServiceNameFromEnvs(strings.Split(string(data), "\x00")); ok {
		return []string{"service:" + svc}
	}
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
