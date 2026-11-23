// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package telemetry

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"

	telemetrydef "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
)

// `error_type` values of forwarder.errors. They reuse the default forwarder's transactions.errors
// values where one exists; `timeout` is added because transactions.errors files timeouts under its
// catch-all.
const (
	ForwarderErrorTypeDNS        = "dns_lookup_failure"
	ForwarderErrorTypeConnection = "connection_failure"
	ForwarderErrorTypeTLS        = "tls_handshake_failure"
	ForwarderErrorTypeTimeout    = "timeout"
	ForwarderErrorTypeOther      = "cant_send"
)

// ForwarderErrors is forwarder.errors, registered once in the process-wide telemetry registry that
// agent telemetry collects: requests to a Datadog intake that got no HTTP response at all. Forwarders
// record them with RecordForwarderTransportError.
var ForwarderErrors = NewForwarderErrors(telemetryimpl.GetCompatComponent())

// NewForwarderErrors registers forwarder.errors in tel. Production code must use ForwarderErrors:
// registering the metric twice in the same registry panics. This is for tests that need their own
// registry.
//
// It mirrors the default forwarder's transactions.errors (domain, endpoint, error_type) with an extra
// `forwarder` tag, the same way forwarder.http_errors mirrors transactions.http_errors.
func NewForwarderErrors(tel telemetrydef.Component) telemetrydef.Counter {
	return tel.NewCounter(
		"forwarder",
		"errors",
		[]string{"domain", "endpoint", "error_type", "forwarder"},
		"Count of requests to Datadog intakes that failed without an HTTP response, per error type and forwarder",
	)
}

// RecordForwarderTransportError records an error returned by http.Client.Do in a forwarder.errors
// counter, classified by ForwarderTransportErrorType. Errors that must not be counted (the caller
// canceled the request) are ignored.
func RecordForwarderTransportError(c telemetrydef.Counter, forwarder, domain, endpoint string, err error) {
	if errorType := ForwarderTransportErrorType(err); errorType != "" {
		c.Inc(domain, endpoint, errorType, forwarder)
	}
}

// ForwarderTransportErrorType returns the `error_type` tag of forwarder.errors for an error returned by
// http.Client.Do (the request got no HTTP response), or "" when the error must not be counted.
//
// Order matters because the cases overlap: a DNS timeout is both a *net.DNSError and a timeout, and a
// dial timeout is both a dial *net.OpError and a timeout.
func ForwarderTransportErrorType(err error) string {
	// The caller gave up (e.g. egress shutting down): the intake was not unreachable.
	if errors.Is(err, context.Canceled) {
		return ""
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return ForwarderErrorTypeDNS
	}

	var netErr net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
		return ForwarderErrorTypeTimeout
	}

	var (
		recordErr   tls.RecordHeaderError
		verifyErr   *tls.CertificateVerificationError
		unknownCA   x509.UnknownAuthorityError
		hostnameErr x509.HostnameError
		invalidCert x509.CertificateInvalidError
	)
	if errors.As(err, &recordErr) || errors.As(err, &verifyErr) || errors.As(err, &unknownCA) ||
		errors.As(err, &hostnameErr) || errors.As(err, &invalidCert) {
		return ForwarderErrorTypeTLS
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) && opErr.Op == "dial" {
		return ForwarderErrorTypeConnection
	}

	return ForwarderErrorTypeOther
}
