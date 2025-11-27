// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

// Package otel provides utilities for the otel.
package otel

import "github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"

// GatewayUsage wraps the attributes.GatewayUsage type to provide gateway usage functionality.
//
// GetHostFromAttributesHandler() handles an important nil check distinction:
// A nil *attributes.GatewayUsage and a nil attributes.HostFromAttributesHandler interface
// are different types, even though both are nil. An interface contains both type information
// and a value, so a nil interface is not equal to a nil concrete type pointer.
//
// This wrapper ensures proper nil handling when converting between the concrete *attributes.GatewayUsage
// type and the attributes.HostFromAttributesHandler interface to avoid nil pointer panics.
type GatewayUsage struct {
	gatewayUsage *attributes.GatewayUsage
}

// NewGatewayUsage creates and returns a new GatewayUsage instance with an initialized underlying gateway usage
func NewGatewayUsage() GatewayUsage {
	return GatewayUsage{
		gatewayUsage: attributes.NewGatewayUsage(),
	}
}

// NewDisabledGatewayUsage creates and returns a new GatewayUsage instance with no underlying gateway usage
func NewDisabledGatewayUsage() GatewayUsage {
	return GatewayUsage{}
}

// GetHostFromAttributesHandler returns a handler for extracting host information from attributes.
func (g GatewayUsage) GetHostFromAttributesHandler() attributes.HostFromAttributesHandler {
	if g.gatewayUsage == nil {
		// return nil is different from return g.gatewayUsage because the type of nil is HostFromAttributesHandler
		return nil
	}
	return g.gatewayUsage
}

// Gauge returns the current gateway usage gauge value and a boolean indicating if gateway usage is enabled.
func (g *GatewayUsage) Gauge() (float64, bool) {
	if g.gatewayUsage != nil {
		return g.gatewayUsage.Gauge(), true
	}
	return 0, false
}
