// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package com_datadoghq_remoteaction_integrations implements Agent integration actions over local IPC.
// This bundle targets the core Agent and must not be registered in the Cluster Agent.
package com_datadoghq_remoteaction_integrations

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"strconv"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipchttp "github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers"
	"github.com/DataDog/datadog-agent/pkg/collector/remotequeries"
	pkgconfighelper "github.com/DataDog/datadog-agent/pkg/config/helper"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

// IntegrationsBundle holds the Agent integration actions.
type IntegrationsBundle struct{ runQuery *RunQuery }

// NewIntegrations creates the local-IPC bundle. Connection credentials are never used.
func NewIntegrations(client ipc.HTTPClient) types.Bundle {
	return &IntegrationsBundle{runQuery: &RunQuery{client: client}}
}

// GetAction returns only the documented runQuery action.
func (b *IntegrationsBundle) GetAction(name string) types.Action {
	if name == "runQuery" {
		return b.runQuery
	}
	return nil
}

// RunQuery resolves or runs a query using the caller's cancellable IPC request.
type RunQuery struct{ client ipc.HTTPClient }

// Run sends only action inputs and valid active action-span context to the core Agent.
func (h *RunQuery) Run(ctx context.Context, task *types.Task, _ *privateconnection.PrivateCredentials) (interface{}, error) {
	if h.client == nil {
		return nil, errors.New("remote queries: IPC client is unavailable")
	}
	if flavor.GetFlavor() == flavor.ClusterAgent {
		return nil, errors.New("remote queries require a core Agent")
	}
	address, err := pkgconfighelper.GetIPCAddress(pkgconfigsetup.Datadog())
	if err != nil {
		return nil, errors.New("remote queries: local IPC address is unavailable")
	}
	input, err := json.Marshal(task.Data.Attributes.Inputs)
	if err != nil {
		return nil, errors.New("remote queries: input is not JSON")
	}
	wire := remotequeries.WireRequest{Input: input}
	if span, ok := telemetry.SpanFromContext(ctx); ok {
		traceID, parentID, priority := span.TraceContext()
		trace := &remotequeries.TraceContext{TraceID: strconv.FormatUint(traceID, 10), ParentID: strconv.FormatUint(parentID, 10), SamplingPriority: priority}
		if trace.Valid() {
			wire.TraceContext = trace
		}
	}
	body, err := json.Marshal(wire)
	if err != nil {
		return nil, errors.New("remote queries: request encoding failed")
	}
	base := "https://" + net.JoinHostPort(address, strconv.Itoa(pkgconfigsetup.Datadog().GetInt("cmd_port")))
	response, err := h.client.Post(base+"/agent/remote-queries", "application/json", bytes.NewReader(body), ipchttp.WithContext(ctx))
	if err != nil {
		return nil, errors.New("remote queries: local IPC call failed")
	}
	resolve, _ := task.Data.Attributes.Inputs["resolveOnly"].(bool)
	answer, err := remotequeries.ParseActionAnswer(response, resolve)
	if err != nil {
		return nil, errors.New("remote queries: core Agent returned a malformed outcome")
	}
	return answer, nil
}
