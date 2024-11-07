// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package traceroute

import (
	"context"
	"encoding/json"
	"net/http"

	sysprobeclient "github.com/DataDog/datadog-agent/cmd/system-probe/api/client"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	clientID = "traceroute-agent-windows"
)

// WindowsTraceroute defines a structure for
// running traceroute from an agent running
// on Windows
type WindowsTraceroute struct {
	cfg            Config
	sysprobeClient *http.Client
}

// New creates a new instance of WindowsTraceroute
// based on an input configuration
func New(cfg Config, _ telemetry.Component) (*WindowsTraceroute, error) {
	log.Debugf("Creating new traceroute with config: %+v", cfg)
	return &WindowsTraceroute{
		cfg: cfg,
		sysprobeClient: &http.Client{
			Transport: &http.Transport{
				DialContext: sysprobeclient.DialContextFunc(pkgconfigsetup.SystemProbe().GetString("system_probe_config.sysprobe_socket")),
			},
		},
	}, nil
}

// Run executes a traceroute
func (w *WindowsTraceroute) Run(_ context.Context) (payload.NetworkPath, error) {
	resp, err := getTraceroute(w.sysprobeClient, clientID, w.cfg.DestHostname, w.cfg.DestPort, w.cfg.Protocol, w.cfg.MaxTTL, w.cfg.Timeout)
	if err != nil {
		return payload.NetworkPath{}, err
	}

	var path payload.NetworkPath
	if err := json.Unmarshal(resp, &path); err != nil {
		return payload.NetworkPath{}, err
	}

	return path, nil
}
