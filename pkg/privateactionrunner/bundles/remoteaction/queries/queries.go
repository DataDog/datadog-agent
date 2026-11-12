// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package queries implements the outcome-only remote queries action over local IPC.
// This bundle targets the core Agent and must not be registered in the Cluster Agent.
package queries

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"net"
	"strconv"
	"time"

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

// Bundle holds the single queries action.
type Bundle struct{ execute *Execute }

// New creates the local-IPC bundle. Connection credentials are never used.
func New(client ipc.HTTPClient) types.Bundle { return &Bundle{execute: &Execute{client: client}} }

// GetAction returns only the documented execute action.
func (b *Bundle) GetAction(name string) types.Action {
	if name == "execute" {
		return b.execute
	}
	return nil
}

// Execute resolves or runs a query using the caller's cancellable IPC request.
type Execute struct{ client ipc.HTTPClient }

// CancelOnRunnerStop ends the IPC call when the runner stops so the core Agent
// can cancel the integration run, including during graceful runner shutdown.
func (*Execute) CancelOnRunnerStop() bool { return true }

// TaskTimeout reads the action's top-level timeout, in whole seconds. Omission
// selects the runner's default. Values that cannot be represented by a Go timer
// fail rather than overflowing into a short or negative deadline.
func (*Execute) TaskTimeout(task *types.Task) (time.Duration, bool, error) {
	value, exists := task.Data.Attributes.Inputs["timeout"]
	if !exists {
		return 0, false, nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return 0, true, errors.New("timeout must be a positive integer in seconds")
	}
	seconds, err := strconv.ParseInt(string(raw), 10, 64)
	if err != nil || seconds <= 0 || seconds > math.MaxInt64/int64(time.Second) {
		return 0, true, errors.New("timeout must be a positive whole number of seconds representable by a timer")
	}
	return time.Duration(seconds) * time.Second, true, nil
}

// Run sends only action inputs and valid active action-span context to the core Agent.
func (h *Execute) Run(ctx context.Context, task *types.Task, _ *privateconnection.PrivateCredentials) (interface{}, error) {
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
