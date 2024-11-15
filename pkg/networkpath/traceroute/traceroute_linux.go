// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

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
	clientID = "traceroute-agent-linux"
)

// LinuxTraceroute defines a structure for
// running traceroute from an agent running
// on Linux
type LinuxTraceroute struct {
	cfg            Config
	sysprobeClient *http.Client
}

// New creates a new instance of LinuxTraceroute
// based on an input configuration
func New(cfg Config, _ telemetry.Component) (*LinuxTraceroute, error) {
	log.Debugf("Creating new traceroute with config: %+v", cfg)
	return &LinuxTraceroute{
		cfg: cfg,
		sysprobeClient: &http.Client{
			Transport: &http.Transport{
				DialContext: sysprobeclient.DialContextFunc(pkgconfigsetup.SystemProbe().GetString("system_probe_config.sysprobe_socket")),
			},
		},
	}, nil
}

// Run executes a traceroute
func (l *LinuxTraceroute) Run(_ context.Context) (payload.NetworkPath, error) {
	resp, err := getTraceroute(l.sysprobeClient, clientID, l.cfg.DestHostname, l.cfg.DestPort, l.cfg.Protocol, l.cfg.MaxTTL, l.cfg.Timeout)
	if err != nil {
		return payload.NetworkPath{}, err
	}

	var path payload.NetworkPath
	if err := json.Unmarshal(resp, &path); err != nil {
		return payload.NetworkPath{}, err
	}

	path.Source.ContainerID = l.cfg.SourceContainerID

	return path, nil
}
