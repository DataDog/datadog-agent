// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm

package http

// Add increments the plain counter since TLS support for Windows is not yet available
func (t *TLSCounter) Add(Transaction) {
	t.counterPlain.Add(1)
}
