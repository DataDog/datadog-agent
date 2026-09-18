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
	authoredscriptssupport "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/authoredscripts"
	authoredscriptsoci "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/authoredscripts/oci"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type RunAuthoredScriptHandler struct {
	catalog                 authoredscriptssupport.Catalog
	artifactResolver        *authoredscriptssupport.ArtifactResolver
	artifactResolverInitErr error
}

// NewRunAuthoredScriptHandler prepares authored-script execution.
func NewRunAuthoredScriptHandler(catalog authoredscriptssupport.Catalog) *RunAuthoredScriptHandler {
	handler := &RunAuthoredScriptHandler{
		catalog: catalog,
	}

	environment := installerenv.FromEnv()
	materializer, err := authoredscriptsoci.NewMaterializer(environment, environment.HTTPClient())
	if err == nil {
		handler.artifactResolver, err = authoredscriptssupport.NewUserArtifactResolver(materializer)
	}
	handler.artifactResolverInitErr = err
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
	if h == nil {
		return nil, errors.New("authored-script handler is not configured")
	}
	if h.catalog == nil {
		return nil, errors.New("authored-script handler is not configured")
	}
	if task == nil || task.Data.Attributes == nil {
		return nil, errors.New("authored-script task is required")
	}

	fqn := task.GetFQN()
	descriptor, err := h.catalog.Lookup(fqn)
	if err != nil {
		return nil, fmt.Errorf("could not look up authored-script package %q: %w", fqn, err)
	}
	if h.artifactResolverInitErr != nil {
		return nil, fmt.Errorf("could not initialize authored-script artifact resolver: %w", h.artifactResolverInitErr)
	}
	if h.artifactResolver == nil {
		return nil, errors.New("authored-script artifact resolver is not configured")
	}

	artifact, err := h.artifactResolver.Resolve(ctx, descriptor)
	if err != nil {
		return nil, fmt.Errorf("could not resolve authored-script package %q: %w", fqn, err)
	}
	scriptPackage, err := authoredscriptssupport.LoadPackage(fqn, descriptor, artifact)
	if err != nil {
		return nil, err
	}

	session, err := authoredscriptssupport.NewSession()
	if err != nil {
		return nil, err
	}
	defer func() {
		if cleanupErr := session.Cleanup(); cleanupErr != nil {
			output = nil
			err = errors.Join(err, cleanupErr)
		}
	}()

	cmd, err := authoredscriptssupport.NewCommand(ctx, scriptPackage, session, task.Data.Attributes.Inputs)
	if err != nil {
		return nil, err
	}
	result, err := authoredscriptssupport.ExecuteCommand(ctx, cmd)
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
