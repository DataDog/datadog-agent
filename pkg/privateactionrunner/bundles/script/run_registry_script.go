// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package com_datadoghq_script

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/authoredscripts"
	authoredscriptsoci "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/authoredscripts/oci"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type RunRegistryScriptHandler struct {
	resolver          *authoredscriptsoci.RegistryResolver
	initErr           error
	allowedRegistries []string
}

func NewRunRegistryScriptHandler(client *http.Client, allowedRegistries []string) *RunRegistryScriptHandler {
	h := &RunRegistryScriptHandler{allowedRegistries: slices.Clone(allowedRegistries)}
	cache, err := os.UserCacheDir()
	if err == nil {
		// Keep registry-authorized packages separate from RC-authorized packages.
		h.resolver, err = authoredscriptsoci.NewRegistryResolver(filepath.Join(cache, "datadog-agent", "registry-authored-scripts"), client)
	}
	h.initErr = err
	return h
}

type RunRegistryScriptInputs struct {
	URI        string                 `json:"uri"`
	Parameters map[string]interface{} `json:"parameters"`
}

type RunRegistryScriptOutputs struct {
	ExitCode       int    `json:"exitCode"`
	Stdout         string `json:"stdout"`
	Stderr         string `json:"stderr"`
	DurationMillis int    `json:"durationMillis"`
}

func (h *RunRegistryScriptHandler) Run(ctx context.Context, task *types.Task, _ *privateconnection.PrivateCredentials) (output interface{}, err error) {
	if task == nil || task.Data.Attributes == nil {
		return nil, errors.New("registry-script task is required")
	}
	inputs, err := types.ExtractInputs[RunRegistryScriptInputs](task)
	if err != nil {
		return nil, err
	}
	if h == nil {
		return nil, errors.New("registry-script handler is not configured")
	}
	if h.initErr != nil {
		return nil, fmt.Errorf("could not initialize registry-script resolver: %w", h.initErr)
	}
	if h.resolver == nil {
		return nil, errors.New("registry-script resolver is not configured")
	}
	pkg, err := h.resolver.Resolve(ctx, inputs.URI, h.allowedRegistries)
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
	cmd, err := authoredscripts.NewCommand(ctx, pkg, session, inputs.Parameters)
	if err != nil {
		return nil, err
	}
	result, err := authoredscripts.ExecuteCommand(ctx, cmd)
	if err != nil {
		return nil, err
	}
	return &RunRegistryScriptOutputs{
		ExitCode:       result.ExitCode,
		Stdout:         result.Stdout,
		Stderr:         result.Stderr,
		DurationMillis: int(result.Duration.Milliseconds()),
	}, nil
}
