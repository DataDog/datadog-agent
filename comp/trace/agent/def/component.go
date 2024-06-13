// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package traceagent provides the agent component type.
package traceagent

import (
	"context"
	"net/http"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes"
	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes/source"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// team: agent-apm

// Component is the agent component type.
type Component interface {
	// SetStatsdClient sets the stats client of the underlying trace agent.
	SetStatsdClient(mclient statsd.ClientInterface)
	// SetOTelAttributeTranslator sets the OTel attributes translator of the underlying trace agent.
	SetOTelAttributeTranslator(attrstrans *attributes.Translator)
	// ReceiveOTLPSpans forwards the OTLP spans to the underlying trace agent to process.
	ReceiveOTLPSpans(ctx context.Context, rspans ptrace.ResourceSpans, httpHeader http.Header) source.Source
	// SendStatsPayload sends a stats payload to the Datadog backend.
	SendStatsPayload(p *pb.StatsPayload)
}
