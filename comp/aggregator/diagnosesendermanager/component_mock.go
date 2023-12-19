// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package diagnosesendermanager defines the sender manager for the local diagnose check
package diagnosesendermanager

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
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
