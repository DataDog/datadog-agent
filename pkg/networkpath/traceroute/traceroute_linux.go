// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package traceroute

import (
	"context"
	"encoding/json"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	dd_config "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/pkg/process/net"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	clientID = "traceroute-agent-linux"
)

// LinuxTraceroute defines a structure for
// running traceroute from an agent running
// on Linux
type LinuxTraceroute struct {
	cfg Config
}

// New creates a new instance of LinuxTraceroute
// based on an input configuration
func New(cfg Config, _ telemetry.Component) (*LinuxTraceroute, error) {
	return &LinuxTraceroute{
		cfg: cfg,
	}, nil
}

// Run executes a traceroute
func (l *LinuxTraceroute) Run(_ context.Context) (payload.NetworkPath, error) {
	tu, err := net.GetRemoteSystemProbeUtil(
		dd_config.SystemProbe.GetString("system_probe_config.sysprobe_socket"))
	if err != nil {
		log.Warnf("could not initialize system-probe connection: %s", err.Error())
		return payload.NetworkPath{}, err
	}

	log.Debugf("Network Path Config: %+v", l.cfg)
	resp, err := tu.GetTraceroute(clientID, l.cfg.DestHostname, l.cfg.DestPort, l.cfg.Protocol, l.cfg.MaxTTL, l.cfg.TimeoutMs)
	if err != nil {
		return payload.NetworkPath{}, err
	}

	var path payload.NetworkPath
	if err := json.Unmarshal(resp, &path); err != nil {
		return payload.NetworkPath{}, err
	}

	return path, nil
}
