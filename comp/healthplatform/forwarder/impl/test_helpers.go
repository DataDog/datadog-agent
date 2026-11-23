// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package forwarderimpl

import (
	"net/http"
	"testing"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// SetTransport makes the forwarders created until the end of the test send their reports with the given transport,
// rather than the one built from the configuration.
func SetTransport(t testing.TB, transport http.RoundTripper) {
	previous := newTransport
	newTransport = func(pkgconfigmodel.Reader) http.RoundTripper { return transport }
	t.Cleanup(func() {
		newTransport = previous
	})
}
