// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build unix

package traceroute

import (
	"context"
	"encoding/json"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"net/http"
)

const (
	clientID = "traceroute-agent-unix"
)

// UnixTraceroute defines a structure for
// running traceroute from an agent running on Unix
type UnixTraceroute struct {
	cfg            config.Config
	sysprobeClient *http.Client
}

// New creates a new instance of UnixTraceroute
// based on an input configuration
func New(cfg config.Config, _ telemetry.Component) (*UnixTraceroute, error) {
	log.Debugf("Creating new traceroute with config: %+v", cfg)

	return &UnixTraceroute{
		cfg:            cfg,
		sysprobeClient: getSysProbeClient(),
	}, nil
}

// Run executes a traceroute
func (l *UnixTraceroute) Run(_ context.Context) (payload.NetworkPath, error) {
	resp, err := getTraceroute(l.sysprobeClient, clientID, l.cfg.DestHostname, l.cfg.DestPort, l.cfg.Protocol, l.cfg.TCPMethod, l.cfg.TCPSynParisTracerouteMode, l.cfg.MaxTTL, l.cfg.Timeout)
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
