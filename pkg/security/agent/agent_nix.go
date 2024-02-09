// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package agent

import (
	"errors"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/dump"
)

// NewRuntimeSecurityAgent instantiates a new RuntimeSecurityAgent
func NewRuntimeSecurityAgent(senderManager sender.SenderManager, hostname string, opts RSAOptions) (*RuntimeSecurityAgent, error) {
	client, err := NewRuntimeSecurityClient()
	if err != nil {
		return nil, err
	}

	// on windows do no telemetry
	telemetry, err := newTelemetry(senderManager, opts.LogProfiledWorkloads, opts.IgnoreDDAgentContainers)
	if err != nil {
		return nil, errors.New("failed to initialize the telemetry reporter")
	}
	// on windows do no storage manager
	storage, err := dump.NewSecurityAgentStorageManager(senderManager)
	if err != nil {
		return nil, err
	}

	return &RuntimeSecurityAgent{
		client:               client,
		hostname:             hostname,
		telemetry:            telemetry,
		storage:              storage,
		running:              atomic.NewBool(false),
		connected:            atomic.NewBool(false),
		eventReceived:        atomic.NewUint64(0),
		activityDumpReceived: atomic.NewUint64(0),
	}, nil
}
