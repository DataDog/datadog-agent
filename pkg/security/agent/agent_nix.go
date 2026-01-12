// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package agent

import (
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/pkg/security/security_profile/storage/backend"
)

// NewRuntimeSecurityAgent instantiates a new RuntimeSecurityAgent
func NewRuntimeSecurityAgent(statsdClient statsd.ClientInterface, hostname string) (*RuntimeSecurityAgent, error) {
	client, err := NewRuntimeSecurityCmdClient()
	if err != nil {
		return nil, err
	}

	// on windows do no storage manager
	storage, err := backend.NewActivityDumpRemoteBackend()
	if err != nil {
		return nil, err
	}

	rsa := &RuntimeSecurityAgent{
		cmdClient:            client,
		statsdClient:         statsdClient,
		hostname:             hostname,
		storage:              storage,
		running:              atomic.NewBool(false),
		connected:            atomic.NewBool(false),
		eventReceived:        atomic.NewUint64(0),
		activityDumpReceived: atomic.NewUint64(0),
	}

	if err = rsa.setupGPRC(); err != nil {
		return nil, err
	}

	return rsa, nil
}
