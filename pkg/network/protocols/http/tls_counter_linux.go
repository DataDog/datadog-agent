// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package http

// Add increments the TLS-aware counter based on the specified transaction's static tags
func (t *TLSCounter) Add(tx Transaction) {
	switch tx.StaticTags() {
	case GnuTLS:
		t.counterGnuTLS.Add(1)
	case OpenSSL:
		t.counterOpenSLL.Add(1)
	case Java:
		t.counterJavaTLS.Add(1)
	case Go:
		t.counterGoTLS.Add(1)
	default:
		t.counterPlain.Add(1)
	}
}
