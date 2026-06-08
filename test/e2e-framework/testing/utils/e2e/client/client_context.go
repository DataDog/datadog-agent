// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

// clientContext is the subset of common.Context needed by the client layer.
// It has no dependency on *testing.T, so non-test callers can implement it.
type clientContext interface {
	Logf(format string, args ...any)
	SessionOutputDir() string
}

// requireNoErr panics if err is non-nil. Used in must-style helpers where
// returning an error would change public API signatures.
func requireNoErr(err error) {
	if err != nil {
		panic(err)
	}
}
