// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package telemetry

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/url"
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	telemetrymock "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
)

// timeoutError is a net.Error that reports a timeout, like the errors returned by a dial deadline.
type timeoutError struct{}

func (timeoutError) Error() string   { return "i/o timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

// clientDoError wraps err the way http.Client.Do does.
func clientDoError(err error) error {
	return &url.Error{Op: "Post", URL: "https://agenthealth-intake.datadoghq.com./api/v2/agenthealth", Err: err}
}

func TestForwarderTransportErrorType(t *testing.T) {
	dial := func(err error) error { return clientDoError(&net.OpError{Op: "dial", Net: "tcp", Err: err}) }
	for name, tc := range map[string]struct {
		err  error
		want string
	}{
		"dns not found":       {dial(&net.DNSError{Err: "no such host", Name: "agenthealth-intake.invalid", IsNotFound: true}), ForwarderErrorTypeDNS},
		"dns timeout":         {dial(&net.DNSError{Err: "i/o timeout", Name: "agenthealth-intake.datadoghq.com", IsTimeout: true}), ForwarderErrorTypeDNS},
		"connection refused":  {dial(&os.SyscallError{Syscall: "connect", Err: syscall.ECONNREFUSED}), ForwarderErrorTypeConnection},
		"network unreachable": {dial(&os.SyscallError{Syscall: "connect", Err: syscall.ENETUNREACH}), ForwarderErrorTypeConnection},
		"dial timeout":        {dial(timeoutError{}), ForwarderErrorTypeTimeout},
		"client timeout":      {clientDoError(context.DeadlineExceeded), ForwarderErrorTypeTimeout},
		"tls not a handshake": {clientDoError(tls.RecordHeaderError{Msg: "first record does not look like a TLS handshake"}), ForwarderErrorTypeTLS},
		"tls unknown CA":      {clientDoError(x509.UnknownAuthorityError{}), ForwarderErrorTypeTLS},
		"tls verification":    {clientDoError(&tls.CertificateVerificationError{Err: x509.UnknownAuthorityError{}}), ForwarderErrorTypeTLS},
		"reset after connect": {clientDoError(&net.OpError{Op: "read", Net: "tcp", Err: &os.SyscallError{Syscall: "read", Err: syscall.ECONNRESET}}), ForwarderErrorTypeOther},
		"unknown":             {clientDoError(errors.New("boom")), ForwarderErrorTypeOther},
		"canceled by caller":  {clientDoError(context.Canceled), ""},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, ForwarderTransportErrorType(tc.err))
		})
	}
}

func TestRecordForwarderTransportError(t *testing.T) {
	tel := telemetrymock.New(t)
	c := NewForwarderErrors(tel)

	refused := clientDoError(&net.OpError{Op: "dial", Net: "tcp", Err: &os.SyscallError{Syscall: "connect", Err: syscall.ECONNREFUSED}})
	RecordForwarderTransportError(c, "fwd", "https://intake.example", "ep", refused)
	RecordForwarderTransportError(c, "fwd", "https://intake.example", "ep", refused)
	RecordForwarderTransportError(c, "fwd", "https://intake.example", "ep", clientDoError(context.DeadlineExceeded))
	// Canceled by the caller: not counted.
	RecordForwarderTransportError(c, "fwd", "https://intake.example", "ep", clientDoError(context.Canceled))

	metrics, err := tel.GetCountMetric("forwarder", "errors")
	require.NoError(t, err)
	got := map[string]float64{}
	for _, m := range metrics {
		assert.Equal(t, "https://intake.example", m.Tags()["domain"])
		assert.Equal(t, "ep", m.Tags()["endpoint"])
		assert.Equal(t, "fwd", m.Tags()["forwarder"])
		got[m.Tags()["error_type"]] = m.Value()
	}
	assert.Equal(t, map[string]float64{ForwarderErrorTypeConnection: 2, ForwarderErrorTypeTimeout: 1}, got)
}
