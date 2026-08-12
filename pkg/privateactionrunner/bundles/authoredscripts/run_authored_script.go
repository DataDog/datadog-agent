// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_authoredscripts

import (
	"context"
	"errors"
	"fmt"
	"runtime"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/artifacts"
	authoredscripts "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/authoredscripts"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// RunAuthoredScriptHandler executes the package authorized for an authored action.
type RunAuthoredScriptHandler struct {
}

func NewRunAuthoredScriptHandler() *RunAuthoredScriptHandler {
	return &RunAuthoredScriptHandler{}
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
	if h == nil {
		return nil, errors.New("authored-script handler is not configured")
	}
	if task == nil || task.Data.Attributes == nil {
		return nil, errors.New("authored-script task is required")
	}
	fqn := task.GetFQN()

	descriptor, err := authoredscripts.NewStaticCatalog().Lookup(fqn, artifacts.Platform{
		OS:   runtime.GOOS,
		Arch: runtime.GOARCH,
	})
	if err != nil {
		return nil, fmt.Errorf("could not resolve artifact for %q: %w", fqn, err)
	}
	// TODO: Replace direct local-store access with the artifact manager when the Fleet downloader is available.
	store, err := artifacts.NewUserCacheStore()
	if err != nil {
		return nil, err
	}
	artifact, err := store.Open(descriptor)
	if err != nil {
		return nil, fmt.Errorf("could not open artifact for %q: %w", fqn, err)
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
