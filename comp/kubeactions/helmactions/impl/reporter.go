// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package helmactionsimpl

import (
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ResultReporter is a stub that logs helm action events.
// TODO: wire to Event Platform once an EVP event type for helm actions is available.
type ResultReporter struct {
	clusterName string
	clusterID   string
}

// NewResultReporter creates a new stub ResultReporter.
func NewResultReporter(clusterName, clusterID string) *ResultReporter {
	return &ResultReporter{
		clusterName: clusterName,
		clusterID:   clusterID,
	}
}

// ReportReceived logs that an action has been received.
func (r *ResultReporter) ReportReceived(actionKey ActionKey, action *HelmAction) {
	log.Infof("[HelmActions] Received action %s (type=%s, requested_by=%s, cluster=%s)",
		actionKey, GetActionType(action), action.RequestedBy, r.clusterName)
}

// ReportResult logs the outcome of an executed action.
func (r *ResultReporter) ReportResult(actionKey ActionKey, _ *HelmAction, result ExecutionResult) {
	log.Infof("[HelmActions] Action %s result: status=%s message=%s",
		actionKey, result.Status, result.Message)
}
