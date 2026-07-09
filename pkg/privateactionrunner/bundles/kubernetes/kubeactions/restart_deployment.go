// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package com_datadoghq_kubernetes_kubeactions

import (
	"context"

	kubeactions "github.com/DataDog/datadog-agent/comp/kubeactions/kubeactions/def"
	kubeactionsimpl "github.com/DataDog/datadog-agent/comp/kubeactions/kubeactions/impl"
	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// RestartDeploymentHandler handles the restart_deployment action.
type RestartDeploymentHandler struct {
	ka kubeactions.Component
}

// NewRestartDeploymentHandler creates a new RestartDeploymentHandler.
func NewRestartDeploymentHandler(ka kubeactions.Component) types.Action {
	return &RestartDeploymentHandler{ka: ka}
}

// Run executes the restart_deployment action.
func (h *RestartDeploymentHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	in, err := types.ExtractInputs[kubeactions.RestartDeploymentInputs](task)
	if err != nil {
		return nil, err
	}
	// The action's identity fixes the kind; it is not a user input.
	in.Kind = "Deployment"
	if err := in.Validate(); err != nil {
		return nil, err
	}

	client, err := support.KubeClient(credential)
	if err != nil {
		return nil, err
	}

	report := newReport(kubeactions.ActionTypeRestartDeployment, in.ResourceRef, task)
	h.ka.ReportReceived(report)

	result := kubeactionsimpl.NewRestartDeploymentExecutor(client).Execute(ctx, in)
	h.ka.ReportResult(report, result)

	return &ActionOutputs{Status: result.Status, Message: result.Message}, nil
}
