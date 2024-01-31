// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !otlp
// +build !otlp

package api

import (
	"github.com/DataDog/datadog-agent/pkg/trace/config"
)

// OTLPReceiver implements an OpenTelemetry Collector receiver which accepts incoming
// data on two ports for both plain HTTP and gRPC.
type OTLPReceiver struct{}

// NewOTLPReceiver returns a new OTLPReceiver which sends any incoming traces down the out channel.
func NewOTLPReceiver(_ chan<- *Payload, _ *config.AgentConfig) *OTLPReceiver {
	return &OTLPReceiver{}
}

// Start starts the OTLPReceiver, if any of the servers were configured as active.
func (o *OTLPReceiver) Start() {
	// NOP
}

// Stop stops any running server.
func (o *OTLPReceiver) Stop() {
	// NOP
}
