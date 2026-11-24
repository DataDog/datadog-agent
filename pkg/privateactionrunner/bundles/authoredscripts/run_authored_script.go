// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_authoredscripts

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	installerenv "github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
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
func NewRunAuthoredScriptHandler(catalog authoredscriptssupport.Catalog, agentHTTPClient *http.Client) *RunAuthoredScriptHandler {
	handler := &RunAuthoredScriptHandler{
		catalog: catalog,
	}

	environment := installerenv.FromEnv()
	materializer, err := authoredscriptsoci.NewMaterializer(environment, agentHTTPClient)
	if err == nil {
		handler.artifactResolver, err = authoredscriptssupport.NewDefaultArtifactResolver(materializer)
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
	if h == nil || h.catalog == nil {
		return nil, errors.New("authored-script handler is not configured")
	}
	if task == nil || task.Data.Attributes == nil {
		return nil, errors.New("authored-script task is required")
	}

	executionFQN := task.GetExecutionFQN()
	logger := log.FromContext(ctx)
	start := time.Now()
	stage := "catalog"
	decision := "denied"
	fields := []log.Field{log.String("execution_fqn", executionFQN)}
	// Register before session cleanup so the audit record includes its outcome.
	defer func() {
		fields = append(fields, log.String("decision", decision), log.Int64("duration_ms", time.Since(start).Milliseconds()))
		if output == nil {
			logger.Warn("Authored script execution completed", append(fields,
				log.String("outcome", "failure"),
				log.String("failure_stage", stage),
			)...)
			return
		}
		logger.Info("Authored script execution completed", append(fields, log.String("outcome", "success"))...)
	}()

	if err := h.catalog.WaitForReady(ctx); err != nil {
		stage = "catalog_readiness"
		return nil, fmt.Errorf("waiting for authored-script catalog: %w", err)
	}
	descriptor, err := h.catalog.Lookup(executionFQN)
	if err != nil {
		if errors.Is(err, authoredscriptssupport.ErrPackageNotConfigured) {
			stage = "package_not_configured"
		}
		return nil, fmt.Errorf("could not look up authored-script package %q: %w", executionFQN, err)
	}
	fields = append(fields,
		log.String("package_name", descriptor.Package),
		log.String("package_version", descriptor.Version),
		log.String("package_digest", "sha256:"+descriptor.SHA256),
	)
	stage = "artifact_resolution"
	if h.artifactResolverInitErr != nil {
		return nil, fmt.Errorf("could not initialize authored-script artifact resolver: %w", h.artifactResolverInitErr)
	}
	if h.artifactResolver == nil {
		return nil, errors.New("authored-script artifact resolver is not configured")
	}

	artifact, err := h.artifactResolver.Resolve(ctx, descriptor)
	if err != nil {
		return nil, fmt.Errorf("could not resolve authored-script package %q: %w", executionFQN, err)
	}
	fields = append(fields, log.Bool("cache_hit", artifact.CacheHit))
	stage = "package_validation"
	scriptPackage, err := authoredscriptssupport.LoadPackage(executionFQN, descriptor, artifact)
	if err != nil {
		return nil, err
	}

	stage = "session_creation"
	session, err := authoredscriptssupport.NewSession()
	if err != nil {
		return nil, err
	}
	defer func() {
		if cleanupErr := session.Cleanup(); cleanupErr != nil {
			if err == nil {
				stage = "session_cleanup"
			}
			output = nil
			err = errors.Join(err, cleanupErr)
		}
	}()

	stage = "command_preparation"
	cmd, err := authoredscriptssupport.NewCommand(ctx, scriptPackage, session, task.Data.Attributes.Inputs)
	if err != nil {
		return nil, err
	}
	stage = "execution"
	decision = "allowed"
	result, err := authoredscriptssupport.ExecuteCommand(ctx, cmd)
	if result.ExitCode >= 0 {
		fields = append(fields, log.Int("exit_code", result.ExitCode))
	}
	if err != nil {
		if result.ExitCode < 0 && errors.Is(err, os.ErrPermission) {
			decision = "denied"
			stage = "host_policy"
		}
		return nil, err
	}

	return &RunAuthoredScriptOutputs{
		ExitCode:       result.ExitCode,
		Stdout:         result.Stdout,
		Stderr:         result.Stderr,
		DurationMillis: int(result.Duration.Milliseconds()),
	}, nil
}
