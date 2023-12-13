// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package http

func (t *Telemetry) countOSSpecific(tx Transaction) {
	switch tx.StaticTags() {
	case GnuTLS:
		t.totalHitsGnuTLS.Add(1)
	case OpenSSL:
		t.totalHitsOpenSLL.Add(1)
	case Java:
		t.totalHitsJavaTLS.Add(1)
	case Go:
		t.totalHitsGoTLS.Add(1)
	default:
		t.totalHitsPlain.Add(1)
	}
}
