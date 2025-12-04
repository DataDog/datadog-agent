// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package traceroute adds traceroute functionality to the agent
package traceroute

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/config"
)

func getTraceroute(ctx context.Context, sysProbeClient *http.Client, clientID string, cfg config.Config) (payload.NetworkPath, error) {
	resp, err := getTracerouteFromSysProbe(ctx, sysProbeClient, clientID, cfg.DestHostname, cfg.DestPort, cfg.Protocol, cfg.TCPMethod, cfg.TCPSynParisTracerouteMode, cfg.DisableWindowsDriver, cfg.ReverseDNS, cfg.MaxTTL, cfg.Timeout, cfg.TracerouteQueries, cfg.E2eQueries)
	if err != nil {
		return payload.NetworkPath{}, fmt.Errorf("error getting traceroute: %v", err)
	}

	var path payload.NetworkPath
	if err = json.Unmarshal(resp, &path); err != nil {
		return payload.NetworkPath{}, fmt.Errorf("error unmarshalling response: %w", err)
	}
	return path, nil
}
