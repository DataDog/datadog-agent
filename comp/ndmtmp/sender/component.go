// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package sender exposes a Sender for netflow.
package sender

import (
	"testing"
	"time"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: network-device-monitoring

// Component is the component type.
type Component interface {
	sender.Sender
}

// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(getDefaultSender),
)

// MockComponent is an interface satisfied by mocksender.MockSender.
type MockComponent interface {
	Component
	AssertServiceCheck(t *testing.T, checkName string, status servicecheck.ServiceCheckStatus, hostname string, tags []string, message string) bool
	AssertMetric(t *testing.T, method string, metric string, value float64, hostname string, tags []string) bool
	AssertMonotonicCount(t *testing.T, method string, metric string, value float64, hostname string, tags []string, flushFirstValue bool) bool
	AssertHistogramBucket(t *testing.T, method string, metric string, value int64, lowerBound float64, upperBound float64, monotonic bool, hostname string, tags []string, flushFirstValue bool) bool
	AssertMetricInRange(t *testing.T, method string, metric string, min float64, max float64, hostname string, tags []string) bool
	AssertMetricTaggedWith(t *testing.T, method string, metric string, tags []string) bool
	AssertMetricNotTaggedWith(t *testing.T, method string, metric string, tags []string) bool
	AssertEvent(t *testing.T, expectedEvent event.Event, allowedDelta time.Duration) bool
	AssertEventPlatformEvent(t *testing.T, expectedRawEvent []byte, expectedEventType string) bool
	AssertEventMissing(t *testing.T, expectedEvent event.Event, allowedDelta time.Duration) bool
}

// MockModule provides a MockSender as the sender Component.
var MockModule = fxutil.Component(
	fx.Provide(
		newMockSender,
		func(s MockComponent) Component {
			return s
		},
	),
)
