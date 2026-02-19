// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_ddagent_logs

import (
	"context"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// TailPodLogHandler implements tailPodLog: reads the last N lines of container logs in a pod.
type TailPodLogHandler struct {
	wmeta workloadmeta.Component
}

// NewTailPodLogHandler creates a new TailPodLogHandler.
func NewTailPodLogHandler(wmeta workloadmeta.Component) *TailPodLogHandler {
	return &TailPodLogHandler{wmeta: wmeta}
}

// Run executes the tailPodLog action.
func (h *TailPodLogHandler) Run(
	_ context.Context,
	task *types.Task,
	_ *privateconnection.PrivateCredentials,
) (interface{}, error) {
	return readPodLogs(h.wmeta, task, tailFile)
}
