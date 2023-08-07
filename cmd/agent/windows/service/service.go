// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package service

import (
	"context"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/cmd/agent/common/signals"
	runcmd "github.com/DataDog/datadog-agent/cmd/agent/subcommands/run"
	dogstatsdServer "github.com/DataDog/datadog-agent/comp/dogstatsd/server"
)

type service struct {
	server dogstatsdServer.Component
}

func NewWindowsService() *service {
	return &service{}
}

func (s *service) Name() string {
	return common.ServiceName
}

func (s *service) Init() error {
	var server dogstatsdServer.Component

	_ = common.CheckAndUpgradeConfig()
	// ignore config upgrade error, continue running with what we have.

	server, err := runcmd.StartAgentWithDefaults()
	if err != nil {
		return err
	}

	s.server = server

	return nil
}

func (s *service) Run(ctx context.Context) error {
	defer runcmd.StopAgentWithDefaults(s.server)

	// Wait for stop signal
	select {
	case <-signals.Stopper:
	case <-signals.ErrorStopper:
	case <-ctx.Done():
	}

	return nil
}
