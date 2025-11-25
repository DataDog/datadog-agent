// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package agent provides the trace agent component type.
package agent

import (
	"context"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes/source"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"

	"go.opentelemetry.io/collector/pdata/ptrace"
)

// team: agent-apm

// Component is the agent component type.
type Component interface {
	// SetOTelAttributeTranslator sets the OTel attributes translator of the underlying trace agent.
	SetOTelAttributeTranslator(attrstrans *attributes.Translator)
	// ReceiveOTLPSpans forwards the OTLP spans to the underlying trace agent to process.
	ReceiveOTLPSpans(ctx context.Context, rspans ptrace.ResourceSpans, httpHeader http.Header, hostFromAttributesHandler attributes.HostFromAttributesHandler) (source.Source, error)
	// SendStatsPayload sends a stats payload to the Datadog backend.
	SendStatsPayload(p *pb.StatsPayload)
	// GetHTTPHandler returns the HTTP handler for the given endpoint.
	GetHTTPHandler(endpoint string) http.Handler
}
