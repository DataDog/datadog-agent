// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

package run

import (
	"context"
	"errors"
	"path/filepath"

	privateactionrunner "github.com/DataDog/datadog-agent/comp/privateactionrunner/def"
	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/servicemain"
)

const (
	// ServiceName is the service name used for the private-action-runner
	ServiceName = "datadog-private-action-runner"
)

var (
	// defaultConfPath is the default configuration file path on Windows
	defaultConfPath = filepath.Join(defaultpaths.ConfPath, "datadog.yaml")
)

type windowsService struct {
	servicemain.DefaultSettings
}

// NewService returns the service entry for the private-action-runner
func NewService() servicemain.Service {
	return &windowsService{}
}

// Name returns the service name for event log records
func (s *windowsService) Name() string {
	return ServiceName
}

// Init implements application initialization, run when SERVICE_START_PENDING.
// This blocks service tools like PowerShell's Start-Service until it returns.
func (s *windowsService) Init() error {
	// Nothing to do during init - configuration loading and fx app startup
	// happens in Run().
	return nil
}

// Run implements all application logic, run when SERVICE_RUNNING.
// The provided context is cancellable - monitor ctx.Done() and return when set.
// The service will exit when Run() returns.
func (s *windowsService) Run(ctx context.Context) error {
	err := runPrivateActionRunner(ctx, defaultConfPath, nil)

	if errors.Is(err, privateactionrunner.ErrNotEnabled) {
		// If private action runner is not enabled, exit cleanly
		return servicemain.ErrCleanStopAfterInit
	}

	return err
}

