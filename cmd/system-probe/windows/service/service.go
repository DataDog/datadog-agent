// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

// Package service implements the Windows Service for the system-probe
package service

import (
	"context"
	"errors"
	"fmt"

	runsubcmd "github.com/DataDog/datadog-agent/cmd/system-probe/subcommands/run"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/servicemain"
)

type service struct {
	servicemain.DefaultSettings
	errChan <-chan error
	ctxChan chan context.Context
}

// NewWindowsService returns the service entry for the system-probe
func NewWindowsService() servicemain.Service {
	return &service{}
}

func (s *service) Name() string {
	return config.ServiceName
}

func (s *service) Init() error {
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

func (s *service) Run(ctx context.Context) error {
	// send context to background agent goroutine so we can stop the system-probe
	s.ctxChan <- ctx
	// wait for system-probe to stop
	return <-s.errChan
}
