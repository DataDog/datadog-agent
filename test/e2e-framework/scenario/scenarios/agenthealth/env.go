// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package agenthealth is the agent-health unified scenario: a VM with the host
// Agent, a dockerized workload, and a fakeintake.
package agenthealth

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/common"
)

// Env is a VM with the host Agent, a dockerized workload, and a fakeintake.
type Env struct {
	RemoteHost *components.RemoteHost
	Agent      *components.RemoteHostAgent
	Fakeintake *components.FakeIntake
	Docker     *components.RemoteHostDocker
}

// Init is called after the environment is provisioned; no extra work needed.
func (e *Env) Init(_ common.Context) error { return nil }
