// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

// Package com_datadoghq_kubernetes_kubeactions implements the
// "com.datadoghq.kubernetes.kubeactions" PAR bundle: the Kubernetes actions
// (delete pod, restart/patch/rollback deployment, get resource) migrated from
// the remote-config-driven pkg/clusteragent/kubeactions subsystem.
package com_datadoghq_kubernetes_kubeactions

import (
	"fmt"

	kubeactions "github.com/DataDog/datadog-agent/comp/kubeactions/kubeactions/def"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// KubernetesKubeActions is the bundle exposing the migrated Kubernetes actions.
type KubernetesKubeActions struct {
	actions map[string]types.Action
}

// NewKubernetesKubeActions builds the bundle, wiring each action to a handler
// that reports lifecycle events through the kubeactions component.
func NewKubernetesKubeActions(ka kubeactions.Component) *KubernetesKubeActions {
	return &KubernetesKubeActions{
		actions: map[string]types.Action{
			kubeactions.ActionNameDeletePod:          NewDeletePodHandler(ka),
			kubeactions.ActionNameRestartDeployment:  NewRestartDeploymentHandler(ka),
			kubeactions.ActionNamePatchDeployment:    NewPatchDeploymentHandler(ka),
			kubeactions.ActionNamePatchDaemonSet:     NewPatchDaemonSetHandler(ka),
			kubeactions.ActionNamePatchStatefulSet:   NewPatchStatefulSetHandler(ka),
			kubeactions.ActionNameRollbackDeployment: NewRollbackDeploymentHandler(ka),
			kubeactions.ActionNameGetResource:        NewGetResourceHandler(ka),
		},
	}
}

// GetAction returns the handler for the named action, or nil if unknown.
func (h *KubernetesKubeActions) GetAction(actionName string) types.Action {
	return h.actions[actionName]
}

// newReport builds an ActionReport from the action type, resource reference and
// task metadata (org ID and job ID).
//
// ActionID prefers the caller-supplied resource.ActionID (the kubernetes-actions
// DB action_id, threaded through wf-actions-server) so the EVP events correlate
// back to that row; it falls back to the PAR job id when not provided.
func newReport(actionType string, r kubeactions.ResourceRef, task *types.Task) kubeactions.ActionReport {
	report := kubeactions.ReportFromResource(actionType, r)
	report.RequestedBy = r.RequestedBy
	if task != nil && task.Data.Attributes != nil {
		report.OrgID = task.Data.Attributes.OrgId
		report.ActionID = task.Data.Attributes.JobId
	}
	if r.ActionID != "" {
		report.ActionID = r.ActionID
	}
	return report
}

// actionErr returns a non-nil error when the executed action did not succeed, so
// the runner publishes task failure to OPMS — matching the EVP action_executed
// result — instead of reporting a failed Kubernetes action as a successful task.
// The terminal EVP event has already been emitted by the caller via ReportResult.
func actionErr(result kubeactions.ExecutionResult) error {
	if result.Status != kubeactions.StatusSuccess {
		return fmt.Errorf("kube action failed: %s", result.Message)
	}
	return nil
}

// reportPreflightFailure emits the terminal failed action_executed event for a
// preflight failure (input validation, Kubernetes client construction) that
// happens before the executor runs, then returns the error. Without it these
// early returns would report nothing to EVP, leaving the kube-actions row stuck
// in its pending state since the writer advances status only from EVP events.
// Callers must have already emitted ReportReceived for the report.
func reportPreflightFailure(ka kubeactions.Component, report kubeactions.ActionReport, err error) (any, error) {
	ka.ReportResult(report, kubeactions.ExecutionResult{
		Status:  kubeactions.StatusFailed,
		Message: err.Error(),
	})
	return nil, err
}
