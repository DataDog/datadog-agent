// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package telemetry defines internal telemetry metrics shared by several components, so that each
// metric is registered once and recorded with the same semantics everywhere.
package telemetry

import (
	"strconv"

	telemetrydef "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
)

// ForwarderHTTPErrors is forwarder.http_errors, registered once in the process-wide telemetry registry
// that agent telemetry collects. Forwarders other than the default forwarder (which has
// transactions.http_errors) record their intake HTTP errors in it with RecordForwarderHTTPError.
var ForwarderHTTPErrors = NewForwarderHTTPErrors(telemetryimpl.GetCompatComponent())

// NewForwarderHTTPErrors registers forwarder.http_errors in tel. Production code must use
// ForwarderHTTPErrors: registering the metric twice in the same registry panics. This is for tests
// that need their own registry.
//
// It mirrors the default forwarder's transactions.http_errors (domain, endpoint, exact status code)
// with an extra `forwarder` tag.
func NewForwarderHTTPErrors(tel telemetrydef.Component) telemetrydef.Counter {
	return tel.NewCounter(
		"forwarder",
		"http_errors",
		[]string{"domain", "endpoint", "code", "forwarder"},
		"Count of HTTP errors from Datadog intakes per http code and forwarder",
	)
}

// RecordForwarderHTTPError records an intake response in a forwarder.http_errors counter, with the
// same semantics as transactions.http_errors: only HTTP errors (status >= 400) are counted, tagged
// with the exact status code. It is a no-op for other statuses.
func RecordForwarderHTTPError(c telemetrydef.Counter, forwarder, domain, endpoint string, statusCode int) {
	if statusCode < 400 {
		return
	}
	c.Inc(domain, endpoint, strconv.Itoa(statusCode), forwarder)
}
