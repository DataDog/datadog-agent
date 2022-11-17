// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows
// +build windows

package probe

import (
	"context"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/probe/winprocmon"
)

type Probe struct {
	config       *config.Config
	statsdClient statsd.ClientInterface

	ctx context.Context
}

func NewProbe(config *config.Config, statsdClient statsd.ClientInterface) (*Probe, error) {
	return &Probe{
		config:       config,
		statsdClient: statsdClient,
	}, nil
}

func (p *Probe) Run() {
	winprocmon.RunLoop(func(evt *winprocmon.WinProcessNotification) {
		p.handleEvent(evt)
	})
}

// handleEvent is essentially a dispatcher for monitoring events
// as of writing, it only supports process events, but should be expanded to
// support multiple types of events via changing `WinProcessNotification` to a generic interface.
func (p *Probe) handleEvent(evt *winprocmon.WinProcessNotification) {
	// From here we call out to process_monitor_windows.go

}
