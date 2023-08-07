// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package run

import (
	"context"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/cmd/agent/common/signals"
	dogstatsdServer "github.com/DataDog/datadog-agent/comp/dogstatsd/server"
)

type service struct {
	dsdServer dogstatsdServer.Component
}

func NewWindowsService() *service {
	return &service{}
}

func (s *service) Name() string {
	return common.ServiceName
}

func (s *service) Init() error {
	var dsdServer dogstatsdServer.Component

	_ = common.CheckAndUpgradeConfig()
	// ignore config upgrade error, continue running with what we have.

	dsdServer, err := StartAgentWithDefaults()
	if err != nil {
		return err
	}

	s.dsdServer = dsdServer

	return nil
}

func (s *service) Run(ctx context.Context) error {
	defer stopAgent(&cliParams{GlobalParams: &command.GlobalParams{}}, s.dsdServer)

	// Wait for stop signal
	select {
	case <-signals.Stopper:
	case <-signals.ErrorStopper:
	case <-ctx.Done():
	}

	return nil
}
