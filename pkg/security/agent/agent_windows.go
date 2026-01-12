// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-go/v5/statsd"
)

// NewRuntimeSecurityAgent instantiates a new RuntimeSecurityAgent
func NewRuntimeSecurityAgent(_ statsd.ClientInterface, hostname string) (*RuntimeSecurityAgent, error) {
	client, err := NewRuntimeSecurityCmdClient()
	if err != nil {
		return nil, err
	}

	// on windows do no telemetry

	rsa := &RuntimeSecurityAgent{
		cmdClient:            client,
		hostname:             hostname,
		storage:              nil,
		running:              atomic.NewBool(false),
		connected:            atomic.NewBool(false),
		eventReceived:        atomic.NewUint64(0),
		activityDumpReceived: atomic.NewUint64(0),
	}

	if err := rsa.setupGPRC(); err != nil {
		return nil, err
	}

	return rsa, nil
}
