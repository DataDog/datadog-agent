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
	traceroute    traceroute.Component
	eventPlatform eventplatform.Component
}

func NewGetNetworkPathHandler(traceroute traceroute.Component, eventPlatform eventplatform.Component) *GetNetworkPathHandler {
	return &GetNetworkPathHandler{
		traceroute:    traceroute,
		eventPlatform: eventPlatform,
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
	// SendToBackend forwards traceroute data to the network path backend.
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
	path.TestRunType = payload.TestRunTypeTriggered
	path.SourceProduct = payload.SourceProductNetworkPath
	path.CollectorType = payload.CollectorTypeAgent
	path.Source.Service = inputs.SourceService
	path.Destination.Service = inputs.DestinationService

	if inputs.SendToBackend {
		if err := h.sendToBackend(path); err != nil {
			return nil, err
		}
	}

	return &path, nil
}

func (h *GetNetworkPathHandler) sendToBackend(path payload.NetworkPath) error {
	if h.eventPlatform == nil {
		return errors.New("event platform forwarder is not available")
	}

	forwarder, ok := h.eventPlatform.Get()
	if !ok {
		return errors.New("event platform forwarder is not available")
	}

	payloadBytes, err := json.Marshal(path)
	if err != nil {
		return fmt.Errorf("failed to marshal network path payload: %w", err)
	}

	msg := message.NewMessage(payloadBytes, nil, "", 0)
	if err := forwarder.SendEventPlatformEventBlocking(msg, eventplatform.EventTypeNetworkPath); err != nil {
		return fmt.Errorf("failed to send network path payload to event platform: %w", err)
	}
	return nil
}
