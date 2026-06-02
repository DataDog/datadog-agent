// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package ddhostnameprocessor

import (
	"context"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/pprofile"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

const hostnameKey = "datadog.host.name"

type ddhostnameProcessor struct {
	host string
}

func (p *ddhostnameProcessor) injectHost(attrs pcommon.Map) {
	if _, ok := attrs.Get(hostnameKey); !ok {
		attrs.PutStr(hostnameKey, p.host)
	}
}

func (p *ddhostnameProcessor) processProfiles(_ context.Context, pd pprofile.Profiles) (pprofile.Profiles, error) {
	if p.host == "" {
		return pd, nil
	}
	rps := pd.ResourceProfiles()
	for i := 0; i < rps.Len(); i++ {
		p.injectHost(rps.At(i).Resource().Attributes())
	}
	return pd, nil
}

func (p *ddhostnameProcessor) processMetrics(_ context.Context, md pmetric.Metrics) (pmetric.Metrics, error) {
	if p.host == "" {
		return md, nil
	}
	rms := md.ResourceMetrics()
	for i := 0; i < rms.Len(); i++ {
		p.injectHost(rms.At(i).Resource().Attributes())
	}
	return md, nil
}

func (p *ddhostnameProcessor) processLogs(_ context.Context, ld plog.Logs) (plog.Logs, error) {
	if p.host == "" {
		return ld, nil
	}
	rls := ld.ResourceLogs()
	for i := 0; i < rls.Len(); i++ {
		p.injectHost(rls.At(i).Resource().Attributes())
	}
	return ld, nil
}

func (p *ddhostnameProcessor) processTraces(_ context.Context, td ptrace.Traces) (ptrace.Traces, error) {
	if p.host == "" {
		return td, nil
	}
	rss := td.ResourceSpans()
	for i := 0; i < rss.Len(); i++ {
		p.injectHost(rss.At(i).Resource().Attributes())
	}
	return td, nil
}
