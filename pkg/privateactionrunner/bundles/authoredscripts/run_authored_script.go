// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package com_datadoghq_authoredscripts

import (
	"context"
	"errors"
	"fmt"

	installerenv "github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/authoredscripts"
	authoredscriptsoci "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/authoredscripts/oci"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type RunAuthoredScriptHandler struct {
	catalog             authoredscripts.Catalog
	packageCache        *authoredscripts.PackageCache
	packageCacheInitErr error
	enabled             bool
}

func NewRunAuthoredScriptHandler(enabled bool) *RunAuthoredScriptHandler {
	handler := &RunAuthoredScriptHandler{
		catalog: authoredscripts.NewStaticCatalog(),
		enabled: enabled,
	}
	if !enabled {
		return handler
	}

	environment := installerenv.FromEnv()
	source, err := authoredscriptsoci.NewSource(environment, environment.HTTPClient())
	if err == nil {
		handler.packageCache, err = authoredscripts.NewUserPackageCache(source)
	}
	handler.packageCacheInitErr = err
	return handler
}

// RunAuthoredScriptOutputs contains the process result returned by an authored action.
type RunAuthoredScriptOutputs struct {
	ExitCode       int    `json:"exitCode"`
	Stdout         string `json:"stdout"`
	Stderr         string `json:"stderr"`
	DurationMillis int    `json:"durationMillis"`
}

func (h *RunAuthoredScriptHandler) Run(
	ctx context.Context,
	task *types.Task,
	_ *privateconnection.PrivateCredentials,
) (output interface{}, err error) {
	if h == nil || h.catalog == nil {
		return nil, errors.New("authored-script handler is not configured")
	}
	if !h.enabled {
		return nil, errAuthoredScriptExecutionNotImplemented
	}
	if task == nil || task.Data.Attributes == nil {
		return nil, errors.New("authored-script task is required")
	}
	fqn := task.GetFQN()
	descriptor, err := h.catalog.Lookup(fqn)
	if err != nil {
		return nil, fmt.Errorf("could not look up authored-script package %q: %w", fqn, err)
	}
	if h.packageCacheInitErr != nil {
		return nil, fmt.Errorf("could not initialize authored-script package cache: %w", h.packageCacheInitErr)
	}
	if h.packageCache == nil {
		return nil, errors.New("authored-script package cache is not configured")
	}

	artifact, err := h.packageCache.Resolve(ctx, descriptor)
	if err != nil {
		return nil, fmt.Errorf("could not resolve authored-script package %q: %w", fqn, err)
	}
	scriptPackage, err := authoredscripts.LoadPackage(fqn, descriptor, artifact)
	if err != nil {
		return nil, err
	}
	session, err := authoredscripts.NewSession()
	if err != nil {
		return nil, err
	}
	defer func() {
		if cleanupErr := session.Cleanup(); cleanupErr != nil {
			output = nil
			err = errors.Join(err, cleanupErr)
		}
	}()
	cmd, err := authoredscripts.NewCommand(ctx, scriptPackage, session, task.Data.Attributes.Inputs)
	if err != nil {
		return nil, err
	}
	result, err := authoredscripts.ExecuteCommand(ctx, cmd)
	if err != nil {
		return nil, err
	}

	return &RunAuthoredScriptOutputs{
		ExitCode:       result.ExitCode,
		Stdout:         result.Stdout,
		Stderr:         result.Stderr,
		DurationMillis: int(result.Duration.Milliseconds()),
	}, nil
}
