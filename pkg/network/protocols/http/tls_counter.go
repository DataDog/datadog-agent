// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (windows && npm) || linux_bpf

package http

import (
	libtelemetry "github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
)

// TLSCounter is a TLS aware counter, it has a plain counter and a counter for each TLS library
// It enables the use of a single metric that increments based on the TLS library, avoiding the need for separate metrics for each TLS library
type TLSCounter struct {
	counterPlain     *libtelemetry.Counter
	counterGnuTLS    *libtelemetry.Counter
	counterOpenSSL   *libtelemetry.Counter
	counterGoTLS     *libtelemetry.Counter
	counterIstioTLS  *libtelemetry.Counter
	counterNodeJSTLS *libtelemetry.Counter
}

// NewTLSCounter creates and returns a new instance of TLSCounter
func NewTLSCounter(metricGroup *libtelemetry.MetricGroup, metricName string, tags ...string) *TLSCounter {
	return &TLSCounter{
		// tls_library:none is a must, as prometheus metrics must have the same cardinality of tags
		counterPlain:     metricGroup.NewCounter(metricName, append(tags, "encrypted:false", "tls_library:none")...),
		counterGnuTLS:    metricGroup.NewCounter(metricName, append(tags, "encrypted:true", "tls_library:gnutls")...),
		counterOpenSSL:   metricGroup.NewCounter(metricName, append(tags, "encrypted:true", "tls_library:openssl")...),
		counterGoTLS:     metricGroup.NewCounter(metricName, append(tags, "encrypted:true", "tls_library:go")...),
		counterIstioTLS:  metricGroup.NewCounter(metricName, append(tags, "encrypted:true", "tls_library:istio")...),
		counterNodeJSTLS: metricGroup.NewCounter(metricName, append(tags, "encrypted:true", "tls_library:nodejs")...),
	}
}
