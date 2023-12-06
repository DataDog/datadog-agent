// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (windows && npm) || linux_bpf

package http

import (
	libtelemetry "github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
)

// A TLSCounter is a TLS aware counter, it has a plain counter and a counter for each TLS library.
type TLSCounter struct {
	counterPlain   *libtelemetry.Counter
	counterGnuTLS  *libtelemetry.Counter
	counterOpenSLL *libtelemetry.Counter
	counterJavaTLS *libtelemetry.Counter
	counterGoTLS   *libtelemetry.Counter
}

func NewTLSCounter(metricGroup *libtelemetry.MetricGroup, metricName string, tags ...string) *TLSCounter {
	return &TLSCounter{
		counterPlain:   metricGroup.NewCounter(metricName, append(tags, "encrypted:false")...),
		counterGnuTLS:  metricGroup.NewCounter(metricName, append(tags, "encrypted:true", "tls_library:gnutls")...),
		counterOpenSLL: metricGroup.NewCounter(metricName, append(tags, "encrypted:true", "tls_library:openssl")...),
		counterJavaTLS: metricGroup.NewCounter(metricName, append(tags, "encrypted:true", "tls_library:java")...),
		counterGoTLS:   metricGroup.NewCounter(metricName, append(tags, "encrypted:true", "tls_library:go")...),
	}
}
