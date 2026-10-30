// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !otlp

package api

import "github.com/DataDog/datadog-agent/pkg/trace/log"

// grpcServer is unused in builds without the otlp tag.
type grpcServer = *struct{}

// Start logs a warning if OTLP ingest is configured, since builds without the otlp tag
// cannot accept OTLP traces over gRPC.
func (o *OTLPReceiver) Start() {
	if o.conf.OTLPReceiver.GRPCPort != 0 {
		log.Warn("OTLP trace ingest over gRPC is not supported in this build; ignoring the configured OTLP gRPC port.")
	}
}

func (o *OTLPReceiver) stopGRPC() {
	// grpcsrv is never set without the otlp tag; reference it so the shared field isn't unused.
	_ = o.grpcsrv
}
