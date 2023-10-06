// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

// Package service implements the Windows Service for the core agent
package service

import (
	"context"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	runcmd "github.com/DataDog/datadog-agent/cmd/agent/subcommands/run"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/servicemain"
)

type service struct {
	errChan <-chan error
	ctxChan chan context.Context
}

// NewWindowsService returns the service entry  for the core agent
func NewWindowsService() servicemain.Service {
	return &service{}
}

func (s *service) Name() string {
	return common.ServiceName
}

func (s *service) Init() error {
	_ = common.CheckAndUpgradeConfig()
	// ignore config upgrade error, continue running with what we have.

	s.ctxChan = make(chan context.Context)

	errChan, err := runcmd.StartAgentWithDefaults(s.ctxChan)
	if err != nil {
		return err
	}

	s.errChan = errChan

	return nil
}

func (s *service) Run(ctx context.Context) error {
	// send context to background agent goroutine so we can stop the agent
	s.ctxChan <- ctx
	// wait for agent to stop
	return <-s.errChan
}
