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

// DeletePodHandler handles the delete_pod action.
type DeletePodHandler struct {
	ka kubeactions.Component
}

// NewDeletePodHandler creates a new DeletePodHandler.
func NewDeletePodHandler(ka kubeactions.Component) types.Action {
	return &DeletePodHandler{ka: ka}
}

// ActionOutputs is the result returned to the Datadog backend for the
// mutating Kubernetes actions (everything except get_resource).
type ActionOutputs struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// Run executes the delete_pod action.
func (h *DeletePodHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	in, err := types.ExtractInputs[kubeactions.DeletePodInputs](task)
	if err != nil {
		return nil, err
	}
	// The action's identity fixes the kind; it is not a user input.
	in.Kind = "Pod"
	if err := in.Validate(); err != nil {
		return nil, err
	}

	client, err := support.KubeClient(credential)
	if err != nil {
		return nil, err
	}

	report := newReport(kubeactions.ActionTypeDeletePod, in.ResourceRef, task)
	h.ka.ReportReceived(report)

	result := kubeactionsimpl.NewDeletePodExecutor(client).Execute(ctx, in)
	h.ka.ReportResult(report, result)

	return &ActionOutputs{Status: result.Status, Message: result.Message}, nil
}
