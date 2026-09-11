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

	artifactoci "github.com/DataDog/datadog-agent/pkg/fleet/installer/artifact/oci"
	installerenv "github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/authoredscripts"
	authoredscriptsoci "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/authoredscripts/oci"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type RunAuthoredScriptHandler struct {
	catalog           authoredscripts.Catalog
	downloader        *authoredscripts.Downloader
	downloaderInitErr error
	enabled           bool
}

// NewRunAuthoredScriptHandler creates the action handler from PAR facilities.
func NewRunAuthoredScriptHandler(dependencies Dependencies) *RunAuthoredScriptHandler {
	handler := &RunAuthoredScriptHandler{catalog: dependencies.Catalog, enabled: dependencies.Enabled}
	if !handler.enabled {
		return handler
	}
	if handler.catalog == nil {
		handler.downloaderInitErr = errors.New("authored-script catalog is required")
		return handler
	}
	environment := installerenv.FromEnv()
	client, err := artifactoci.NewClient(environment, environment.HTTPClient())
	if err == nil {
		var materializer *authoredscriptsoci.Materializer
		materializer, err = authoredscriptsoci.NewMaterializer(client)
		if err == nil {
			handler.downloader, err = authoredscripts.NewUserCacheDownloader(materializer.Platform(), materializer)
		}
	}
	handler.downloaderInitErr = err
	return handler
}

// RunAuthoredScriptOutputs is the serialized result of an authored script.
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
	if !h.enabled {
		return nil, errAuthoredScriptExecutionDisabled
	}
	if task == nil || task.Data.Attributes == nil {
		return nil, errors.New("authored-script task is required")
	}
	if h.catalog == nil {
		return nil, errors.New("authored-script catalog is not configured")
	}
	if h.downloaderInitErr != nil {
		return nil, fmt.Errorf("could not initialize authored-script downloader: %w", h.downloaderInitErr)
	}
	if h.downloader == nil {
		return nil, errors.New("authored-script downloader is not configured")
	}

	fqn := task.GetFQN()
	descriptor, err := h.catalog.Lookup(fqn)
	if err != nil {
		return nil, fmt.Errorf("could not resolve authored-script package %q: %w", fqn, err)
	}
	localArtifact, err := h.downloader.Download(ctx, descriptor)
	if err != nil {
		return nil, fmt.Errorf("could not download authored-script package %q: %w", fqn, err)
	}
	scriptPackage, err := authoredscripts.LoadPackage(fqn, descriptor, localArtifact)
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
	result, err := authoredscripts.ExecuteCommandWithAuthorization(ctx, cmd, func(start func() error) error {
		return h.catalog.WithAuthorized(descriptor, start)
	})
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
