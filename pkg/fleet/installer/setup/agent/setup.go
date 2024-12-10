// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package agent setups the agent
package agent

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/setup/common"
)

type agentSetup struct {
	*common.Setup
}

func Setup(ctx context.Context, env *env.Env) error {
	s, err := common.NewSetup(env)
	if err != nil {
		return err
	}
	setup := &agentSetup{
		Setup: s,
	}
	return nil
}

func (as *agentSetup) setup() error {
	return nil
}
