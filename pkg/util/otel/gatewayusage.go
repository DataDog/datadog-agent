// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

// Package otel provides utilities for the otel.
package otel

import (
	"sync/atomic"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
)

// GatewayUsage wraps the attributes.GatewayUsage type to provide gateway usage functionality and
// reading env variable DD_OTELCOLLECTOR_GATEWAY_MODE (set by helm chart or operator) to indicate
// usage of gateway mode.
// Env. variable has priority over attributes!
// GetHostFromAttributesHandler() handles an important nil check distinction:
// A nil *attributes.GatewayUsage and a nil attributes.HostFromAttributesHandler interface
// are different types, even though both are nil. An interface contains both type information
// and a value, so a nil interface is not equal to a nil concrete type pointer.
//
// This wrapper ensures proper nil handling when converting between the concrete *attributes.GatewayUsage
// type and the attributes.HostFromAttributesHandler interface to avoid nil pointer panics.
type GatewayUsage struct {
	gatewayUsageAttr *attributes.GatewayUsage
	gatewayModeEnv   *atomic.Bool
}

// NewGatewayUsage creates and returns a new GatewayUsage instance with an initialized underlying gateway usage
func NewGatewayUsage(gatewayModeSet bool) GatewayUsage {
	gatewayModeEnv := &atomic.Bool{}

	if gatewayModeSet {
		gatewayModeEnv.Store(true)
	}

	return GatewayUsage{
		gatewayUsageAttr: attributes.NewGatewayUsage(),
		gatewayModeEnv:   gatewayModeEnv,
	}
}

// NewDisabledGatewayUsage creates and returns a new GatewayUsage instance with no underlying gateway usage
func NewDisabledGatewayUsage() GatewayUsage {
	return GatewayUsage{}
}

// GetHostFromAttributesHandler returns a handler for extracting host information from attributes.
func (g GatewayUsage) GetHostFromAttributesHandler() attributes.HostFromAttributesHandler {
	if g.gatewayUsageAttr == nil {
		// return nil is different from return g.gatewayUsage because the type of nil is HostFromAttributesHandler
		return nil
	}
	return g.gatewayUsageAttr
}

// Gauge returns the value of gateway env. variable
// 1 - set to true
// 0 - set to false or not set
func (g *GatewayUsage) EnvVarValue() float64 {
	if g.gatewayModeEnv != nil && g.gatewayModeEnv.Load() {
		return 1
	}

	return 0
}

// Gauge returns the current gateway usage gauge value and a boolean indicating if gateway usage is enabled.
func (g *GatewayUsage) Gauge() (float64, bool) {
	if g.gatewayModeEnv != nil && g.gatewayModeEnv.Load() {
		return 1, true
	}

	if g.gatewayUsageAttr != nil {
		return g.gatewayUsageAttr.Gauge(), true
	}
	return 0, false
}
