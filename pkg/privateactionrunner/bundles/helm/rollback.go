// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package com_datadoghq_helm

import (
	"context"
	"fmt"

	helmactions "github.com/DataDog/datadog-agent/comp/kubeactions/helmactions/def"
	helmactionsimpl "github.com/DataDog/datadog-agent/comp/kubeactions/helmactions/impl"
	kubeactions "github.com/DataDog/datadog-agent/comp/kubeactions/kubeactions/def"
	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	batchv1 "k8s.io/api/batch/v1"
)

type HelmRollbackHandler struct {
	ha helmactions.Component
	ka kubeactions.Component
}

func NewRollbackHandler(ha helmactions.Component, ka kubeactions.Component) types.Action {
	return &HelmRollbackHandler{
		ha: ha,
		ka: ka,
	}
}

type HelmRollbackOutputs struct {
	Job *batchv1.Job
}

// Run returns any which is the serialized and send back to DD backend
func (rh *HelmRollbackHandler) Run(ctx context.Context, task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	in, err := types.ExtractInputs[helmactions.RollbackInputs](task)
	if err != nil {
		return nil, err
	}

	report := newReport(helmactions.HelmRollbackAction, task)
	report.ResourceNamespace = in.ReleaseNamespace
	report.ResourceName = in.Release

	// Inform KA backend about job reception
	rh.ka.ReportReceived(report)

	client, err := support.KubeClient(credential)
	if err != nil {
		return rh.reportPreflightFailure(report, err)
	}

	job, err := helmactionsimpl.NewRollbackExecutor(client).Run(ctx, in)
	if err != nil {
		return rh.reportPreflightFailure(report, fmt.Errorf("helm rollback executor: %w", err))
	}

	// Since Helm Rollback is long running action report progress event that it has started
	rh.ka.ReportProgress(report, "Rollback started")
	// Notify helm actions tracking
	rh.ha.OnRollback(&in, job)

	return &HelmRollbackOutputs{
		Job: job,
	}, nil
}

// reportPreflightFailure emits the terminal failed action_executed event for a
// preflight failure (input validation, Kubernetes client construction) that
// happens before the executor runs, then returns the error. Without it these
// early returns would report nothing to EVP, leaving the kube-actions row stuck
// in its pending state since the writer advances status only from EVP events.
// Callers musцt have already emitted ReportReceived for the report.
func (rh *HelmRollbackHandler) reportPreflightFailure(report kubeactions.ActionReport, err error) (any, error) {
	rh.ka.ReportResult(report, kubeactions.ExecutionResult{
		Status:  kubeactions.StatusFailed,
		Message: err.Error(),
	})
	return nil, err
}

// newReport builds an ActionReport from the action type and task metadata (org ID and job ID).
//
// ActionID is taken from TaskID since if created using kube_actions API it is so.
func newReport(actionType string, task *types.Task) kubeactions.ActionReport {
	report := kubeactions.ActionReport{
		ActionType: actionType,
	}

	if task != nil && task.Data.Attributes != nil {
		report.ActionID = task.Data.ID
		report.OrgID = task.Data.Attributes.OrgId
	}
	return report
}
