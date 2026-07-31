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

// PatchDaemonSetHandler handles the patch_daemonset action.
type PatchDaemonSetHandler struct {
	ka kubeactions.Component
}

// NewPatchDaemonSetHandler creates a new PatchDaemonSetHandler.
func NewPatchDaemonSetHandler(ka kubeactions.Component) types.Action {
	return &PatchDaemonSetHandler{ka: ka}
}

// Run executes the patch_daemonset action.
func (h *PatchDaemonSetHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	in, err := types.ExtractInputs[kubeactions.PatchDaemonSetInputs](task)
	if err != nil {
		return nil, err
	}
	// The action's identity fixes the kind; it is not a user input.
	in.Kind = "DaemonSet"
	if err := in.Validate(); err != nil {
		return nil, err
	}

	client, err := support.KubeClient(credential)
	if err != nil {
		return nil, err
	}

	report := newReport(kubeactions.ActionTypePatchDaemonSet, in.ResourceRef, task)
	h.ka.ReportReceived(report)

	result := kubeactionsimpl.NewPatchDaemonSetExecutor(client).Execute(ctx, in)
	h.ka.ReportResult(report, result)

	if err := actionErr(result); err != nil {
		return nil, err
	}
	return &ActionOutputs{Status: result.Status, Message: result.Message}, nil
}
