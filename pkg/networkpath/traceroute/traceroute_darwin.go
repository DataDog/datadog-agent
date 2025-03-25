// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package traceroute

import (
	"context"
	"errors"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/config"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/runner"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	tcpNotSupportedMsg = "TCP traceroute is not currently supported on macOS"
)

// MacTraceroute defines a structure for
// running traceroute from an agent running
// on macOS
type MacTraceroute struct {
	cfg    config.Config
	runner *runner.Runner
}

// New creates a new instance of MacTraceroute
// based on an input configuration
func New(cfg config.Config, telemetry telemetry.Component) (*MacTraceroute, error) {
	log.Debugf("Creating new traceroute with config: %+v", cfg)
	// this should use fx dependency injection, but that requires properly passing hostnameComp
	// through both the core agent and process agent, which turns out to be difficult.
	// in addition, we only expect this to run on the core agent, not the process agent, since
	// CNM is not supported on macOS
	// TODO refactor traceroute dependencies?
	runner, err := runner.New(telemetry, hostnameimpl.NewHostnameService())
	if err != nil {
		return nil, err
	}

	// TCP is not supported at the moment due to the
	// way go listens for TCP in our implementation on BSD systems
	if cfg.Protocol == payload.ProtocolTCP {
		return nil, errors.New(tcpNotSupportedMsg)
	}

	return &MacTraceroute{
		cfg:    cfg,
		runner: runner,
	}, nil
}

// Run executes a traceroute
func (m *MacTraceroute) Run(ctx context.Context) (payload.NetworkPath, error) {
	// TODO: mac implementation, can we get this no system-probe or root access?
	// To test: we probably can, but maybe not without modifying
	// the library we currently use

	// TCP is not supported at the moment due to the
	// way go listens for TCP in our implementation on BSD systems
	if m.cfg.Protocol == payload.ProtocolTCP {
		return payload.NetworkPath{}, errors.New(tcpNotSupportedMsg)
	}

	return m.runner.RunTraceroute(ctx, m.cfg)
}
