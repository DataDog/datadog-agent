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

	installerenv "github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	authoredscriptssupport "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/authoredscripts"
	authoredscriptsoci "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/authoredscripts/oci"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/observability"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type RunAuthoredScriptHandler struct {
	catalog                 authoredscriptssupport.Catalog
	artifactResolver        *authoredscriptssupport.ArtifactResolver
	artifactResolverInitErr error
	newSession              func() (*authoredscriptssupport.Session, error)
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
	if h == nil {
		return nil, errors.New("authored-script handler is not configured")
	}
	if task == nil || task.Data.Attributes == nil {
		return nil, errors.New("authored-script task is required")
	}
	audit := authoredScriptAudit{Stage: "configuration"}
	// run returns only after its session cleanup. The shared executor owns
	// lifecycle reporting; the action only contributes safe metadata.
	output, err = h.run(ctx, task, &audit)
	fields := append(audit.Fields(),
		log.String("execution_fqn", task.GetExecutionFQN()),
		log.String("execution_kind", "authored_script"),
	)
	observability.SetExecutionDetails(ctx, audit.Stage, fields...)
	return output, err
}

func (h *RunAuthoredScriptHandler) run(ctx context.Context, task *types.Task, audit *authoredScriptAudit) (output interface{}, err error) {
	if h.catalog == nil {
		return nil, errors.New("authored-script handler is not configured")
	}

	audit.Stage = "catalog"
	executionFQN := task.GetExecutionFQN()
	if err := h.catalog.WaitForReady(ctx); err != nil {
		return nil, fmt.Errorf("waiting for authored-script catalog: %w", err)
	}
	descriptor, err := h.catalog.Lookup(executionFQN)
	if err != nil {
		return nil, fmt.Errorf("could not look up authored-script package %q: %w", executionFQN, err)
	}
	audit.PackageName = descriptor.Package
	audit.PackageVersion = descriptor.Version
	audit.PackageDigest = "sha256:" + descriptor.SHA256
	audit.Stage = "artifact_resolution"
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
	audit.Stage = "package_validation"
	scriptPackage, err := authoredscriptssupport.LoadPackage(executionFQN, descriptor, artifact)
	if err != nil {
		return nil, err
	}

	audit.Stage = "session_creation"
	newSession := h.newSession
	if newSession == nil {
		newSession = authoredscriptssupport.NewSession
	}
	session, err := newSession()
	if err != nil {
		return nil, err
	}
	defer func() {
		if cleanupErr := session.Cleanup(); cleanupErr != nil {
			audit.Stage = "session_cleanup"
			output = nil
			err = errors.Join(err, cleanupErr)
		}
	}()

	audit.Stage = "command_preparation"
	cmd, err := authoredscriptssupport.NewCommand(ctx, scriptPackage, session, task.Data.Attributes.Inputs)
	if err != nil {
		return nil, err
	}
	audit.Stage = "execution"
	result, err := authoredscriptssupport.ExecuteCommand(ctx, cmd)
	if result.ExitCode >= 0 {
		audit.ExitCode = &result.ExitCode
	}
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
