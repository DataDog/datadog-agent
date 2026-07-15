// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package run

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/serializer/types"
)

// noopSerializer satisfies serializer.MetricSerializer without a forwarder/compression
// pipeline. Standalone host-profiler only wires dogtelextension for its local
// workloadmeta-backed tagger; the extension's liveness metric has nowhere useful to go
// here, and its failure is already non-fatal (logged as a warning by the extension).
type noopSerializer struct{}

var _ serializer.MetricSerializer = (*noopSerializer)(nil)

func (*noopSerializer) SendEvents(event.Events) error                    { return nil }
func (*noopSerializer) SendServiceChecks(servicecheck.ServiceChecks) error { return nil }
func (*noopSerializer) SendIterableSeries(metrics.SerieSource) error      { return nil }
func (*noopSerializer) AreSeriesEnabled() bool                            { return false }
func (*noopSerializer) SendSketch(metrics.SketchesSource) error           { return nil }
func (*noopSerializer) AreSketchesEnabled() bool                          { return false }
func (*noopSerializer) SendMetadata(marshaler.JSONMarshaler) error        { return nil }
func (*noopSerializer) SendHostMetadata(marshaler.JSONMarshaler) error    { return nil }
func (*noopSerializer) SendProcessesMetadata(interface{}) error           { return nil }
func (*noopSerializer) SendAgentchecksMetadata(marshaler.JSONMarshaler) error {
	return nil
}
func (*noopSerializer) SendOrchestratorMetadata([]types.ProcessMessageBody, string, string, int) error {
	return nil
}
func (*noopSerializer) SendOrchestratorManifests([]types.ProcessMessageBody, string, string) error {
	return nil
}
func (*noopSerializer) SendAgentShutdownEvent(context.Context, *event.Event) error {
	return nil
}
