// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"go.uber.org/atomic"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-go/v5/statsd"
)

// NewRuntimeSecurityAgent instantiates a new RuntimeSecurityAgent
func NewRuntimeSecurityAgent(_ statsd.ClientInterface, hostname string, _ RSAOptions, _ workloadmeta.Component) (*RuntimeSecurityAgent, error) {
	client, err := NewRuntimeSecurityClient()
	if err != nil {
		return nil, err
	}

	// on windows do no telemetry

	return &RuntimeSecurityAgent{
		client:               client,
		hostname:             hostname,
		telemetry:            nil,
		storage:              nil,
		running:              atomic.NewBool(false),
		connected:            atomic.NewBool(false),
		eventReceived:        atomic.NewUint64(0),
		activityDumpReceived: atomic.NewUint64(0),
	}, nil
}
