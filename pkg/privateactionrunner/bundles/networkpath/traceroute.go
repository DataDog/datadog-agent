// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_networkpath

import (
	"context"
	"fmt"
	"time"

	traceroute "github.com/DataDog/datadog-agent/comp/networkpath/traceroute/def"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// TracerouteHandler handles the traceroute action.
type TracerouteHandler struct {
	traceroute traceroute.Component
}

// NewTracerouteHandler creates a new TracerouteHandler with the given traceroute component.
func NewTracerouteHandler(tr traceroute.Component) *TracerouteHandler {
	return &TracerouteHandler{
		traceroute: tr,
	}
}

// TracerouteInputs defines the input parameters for the traceroute action.
type TracerouteInputs struct {
	Hostname           string            `json:"hostname"`
	Port               uint16            `json:"port"`
	SourceService      string            `json:"sourceService"`
	DestinationService string            `json:"destinationService"`
	MaxTTL             uint8             `json:"maxTTL"`
	Protocol           payload.Protocol  `json:"protocol"`
	TCPMethod          payload.TCPMethod `json:"tcpMethod"`
	Timeout            time.Duration     `json:"timeout"`
	TracerouteQueries  int               `json:"tracerouteQueries"`
	E2eQueries         int               `json:"e2eQueries"`
	Namespace          string            `json:"namespace"`
}

// TracerouteOutputs defines the output of the traceroute action.
type TracerouteOutputs struct {
	Path payload.NetworkPath `json:"path"`
}

// Run executes the traceroute action.
func (h *TracerouteHandler) Run(
	ctx context.Context,
	task *types.Task,
	_ *privateconnection.PrivateCredentials,
) (interface{}, error) {
	inputs, err := types.ExtractInputs[TracerouteInputs](task)
	if err != nil {
		return nil, err
	}

	cfg := config.Config{
		DestHostname:       inputs.Hostname,
		DestPort:           inputs.Port,
		SourceService:      inputs.SourceService,
		DestinationService: inputs.DestinationService,
		MaxTTL:             inputs.MaxTTL,
		Protocol:           inputs.Protocol,
		TCPMethod:          inputs.TCPMethod,
		Timeout:            inputs.Timeout,
		TracerouteQueries:  inputs.TracerouteQueries,
		E2eQueries:         inputs.E2eQueries,
		ReverseDNS:         true,
	}

	path, err := h.traceroute.Run(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to trace path: %w", err)
	}

	err = payload.ValidateNetworkPath(&path)
	if err != nil {
		return nil, fmt.Errorf("failed to validate network path: %w", err)
	}

	path.Namespace = inputs.Namespace
	path.Source.Service = inputs.SourceService
	path.Destination.Service = inputs.DestinationService

	return &TracerouteOutputs{
		Path: path,
	}, nil
}
