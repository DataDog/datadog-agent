// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_ddagent_networkpath

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	traceroute "github.com/DataDog/datadog-agent/comp/networkpath/traceroute/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	tracerouteconfig "github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type GetNetworkPathHandler struct {
	traceroute  traceroute.Component
	epForwarder eventplatform.Forwarder
}

func NewGetNetworkPathHandler(traceroute traceroute.Component, epComponent eventplatform.Component) *GetNetworkPathHandler {
	var epForwarder eventplatform.Forwarder
	if epComponent != nil {
		epForwarder, _ = epComponent.Get()
	}

	return &GetNetworkPathHandler{
		traceroute:  traceroute,
		epForwarder: epForwarder,
	}
}

type GetNetworkPathInputs struct {
	Hostname           string            `json:"hostname"`
	Port               uint16            `json:"port"`
	SourceService      string            `json:"sourceService,omitempty"`
	DestinationService string            `json:"destinationService,omitempty"`
	MaxTTL             uint8             `json:"maxTtl,omitempty"`
	Protocol           payload.Protocol  `json:"protocol,omitempty"`
	TCPMethod          payload.TCPMethod `json:"tcpMethod,omitempty"`
	TimeoutMs          int64             `json:"timeoutMs,omitempty"`
	TracerouteQueries  int               `json:"tracerouteQueries,omitempty"`
	E2eQueries         int               `json:"e2eQueries,omitempty"`
	Namespace          string            `json:"namespace,omitempty"`
	// Sent traceroute data to network path backend.
	SendToBackend bool `json:"sendToBackend,omitempty"`
}

func (h *GetNetworkPathHandler) Run(
	ctx context.Context,
	task *types.Task,
	_ *privateconnection.PrivateCredentials,
) (interface{}, error) {
	if h.traceroute == nil {
		return nil, errors.New("traceroute component is not available")
	}

	inputs, err := types.ExtractInputs[GetNetworkPathInputs](task)
	if err != nil {
		return nil, err
	}

	protocol := inputs.Protocol
	if protocol == "" {
		protocol = payload.ProtocolUDP
	}

	maxTTL := inputs.MaxTTL
	if maxTTL == 0 {
		maxTTL = pkgconfigsetup.DefaultNetworkPathMaxTTL
	}

	timeout := time.Duration(inputs.TimeoutMs) * time.Millisecond
	if timeout == 0 {
		timeout = pkgconfigsetup.DefaultNetworkPathTimeout * time.Millisecond
	}

	tracerouteQueries := inputs.TracerouteQueries
	if tracerouteQueries == 0 {
		tracerouteQueries = pkgconfigsetup.DefaultNetworkPathStaticPathTracerouteQueries
	}

	e2eQueries := inputs.E2eQueries
	if e2eQueries == 0 {
		e2eQueries = pkgconfigsetup.DefaultNetworkPathStaticPathE2eQueries
	}

	cfg := tracerouteconfig.Config{
		DestHostname:       inputs.Hostname,
		DestPort:           inputs.Port,
		DestinationService: inputs.DestinationService,
		SourceService:      inputs.SourceService,
		MaxTTL:             maxTTL,
		Timeout:            timeout,
		Protocol:           protocol,
		TCPMethod:          inputs.TCPMethod,
		ReverseDNS:         true,
		TracerouteQueries:  tracerouteQueries,
		E2eQueries:         e2eQueries,
	}

	path, err := h.traceroute.Run(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to trace path: %w", err)
	}

	if err := payload.ValidateNetworkPath(&path); err != nil {
		return nil, fmt.Errorf("failed to validate network path: %w", err)
	}

	path.Namespace = inputs.Namespace
	path.Origin = payload.PathOriginNetworkPathIntegration
	path.TestRunType = payload.TestRunTypeOnDemand
	path.SourceProduct = payload.SourceProductNetworkPath
	path.CollectorType = payload.CollectorTypeAgent
	path.Source.Service = inputs.SourceService
	path.Destination.Service = inputs.DestinationService

	if inputs.SendToBackend {
		if err := h.SendNetPathMDToEP(path); err != nil {
			return nil, fmt.Errorf("failed to send network path metadata: %w", err)
		}
	}

	return &path, nil
}

// SendNetPathMDToEP sends a traced network path to Event Platform
func (h *GetNetworkPathHandler) SendNetPathMDToEP(path payload.NetworkPath) error {
	if h.epForwarder == nil {
		return errors.New("event platform forwarder is not available")
	}

	payloadBytes, err := json.Marshal(path)
	if err != nil {
		return fmt.Errorf("error marshalling device metadata: %w", err)
	}

	msg := message.NewMessage(payloadBytes, nil, "", 0)
	if err := h.epForwarder.SendEventPlatformEventBlocking(msg, eventplatform.EventTypeNetworkPath); err != nil {
		return fmt.Errorf("error sending metadata to event platform intake: %w", err)
	}

	return nil
}
