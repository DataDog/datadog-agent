// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package kubeactions provides a component for executing Kubernetes actions
// (delete pod, restart/patch/rollback deployment, get resource) on behalf of
// the cluster agent, and for reporting their progress and results to the
// backend via the Event Platform.
//
// Actions are driven by the Private Action Runner (PAR): each action is a
// workflow task routed to a handler under the
// "com.datadoghq.kubernetes.kubeactions" bundle. The PAR only reports back to
// the backend when a task completes, so the reporting methods exposed here let
// handlers emit intermediate updates (action_received, action_progress) in
// addition to the final result (action_executed).
package kubeactions

// team: container-integrations

// Component is the component type. It exposes Event Platform reporting helpers
// so PAR action handlers can emit lifecycle events in a single call.
type Component interface {
	// ReportReceived emits an "action_received" event when a handler starts
	// processing an action.
	ReportReceived(report ActionReport)
	// ReportProgress emits an intermediate "action_progress" event. The PAR
	// itself only reports on completion, so this is how handlers surface
	// progress for long-running actions.
	ReportProgress(report ActionReport, message string)
	// ReportResult emits the terminal "action_executed" event carrying the
	// execution outcome.
	ReportResult(report ActionReport, result ExecutionResult)
}
