// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

// Package service implements the system-probe Windows service
package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	runsubcmd "github.com/DataDog/datadog-agent/cmd/system-probe/subcommands/run"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/servicemain"
)

// Service implements the system-probe Windows service
type Service struct {
	servicemain.DefaultSettings
	errChan <-chan error
	ctxChan chan context.Context
}

// Name returns the service name
func (s *Service) Name() string {
	return config.ServiceName
}

// Init initializes the service
func (s *Service) Init() error {
	s.ctxChan = make(chan context.Context)

	errChan, err := runsubcmd.StartSystemProbeWithDefaults(s.ctxChan)
	if err != nil {
		if errors.Is(err, runsubcmd.ErrNotEnabled) {
			return fmt.Errorf("%w: %w", servicemain.ErrCleanStopAfterInit, err)
		}
		return err
	}

	s.errChan = errChan

	return nil
}

// Run the service
func (s *Service) Run(ctx context.Context) error {
	// send context to background agent goroutine so we can stop the agent
	s.ctxChan <- ctx
	// wait for agent to stop
	return <-s.errChan
}
