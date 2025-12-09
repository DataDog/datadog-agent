// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package localimpl implements the traceroute component interface
package localimpl

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/hostname"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	traceroute "github.com/DataDog/datadog-agent/comp/networkpath/traceroute/def"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/config"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/runner"
)

// Requires defines the dependencies for the traceroute component
type Requires struct {
	Lifecycle compdef.Lifecycle

	Telemetry telemetry.Component
	Hostname  hostname.Component
}

// Provides defines the output of the traceroute component
type Provides struct {
	Comp traceroute.Component
}

type localTraceroute struct {
	runner   *runner.Runner
	hostname hostname.Component
}

// NewComponent creates a new traceroute component
func NewComponent(reqs Requires) (Provides, error) {
	tracerouteRunner, err := runner.New(reqs.Telemetry)
	if err != nil {
		return Provides{}, err
	}

	reqs.Lifecycle.Append(compdef.Hook{
		OnStart: func(_ctx context.Context) error {
			tracerouteRunner.Start()
			return nil
		},
	})

	lt := &localTraceroute{
		runner:   tracerouteRunner,
		hostname: reqs.Hostname,
	}

	provides := Provides{Comp: lt}
	return provides, nil
}

func (t *localTraceroute) Run(ctx context.Context, cfg config.Config) (payload.NetworkPath, error) {
	path, err := t.runner.Run(ctx, cfg)
	if err != nil {
		return payload.NetworkPath{}, fmt.Errorf("error getting traceroute: %s", err)
	}

	agentHostname, err := t.hostname.Get(ctx)
	if err != nil {
		return payload.NetworkPath{}, fmt.Errorf("error getting the hostname: %w", err)
	}
	path.Source.Hostname = agentHostname
	path.Source.ContainerID = cfg.SourceContainerID

	return path, nil
}
