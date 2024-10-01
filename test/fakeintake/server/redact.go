// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package server implements a dummy http Datadog intake, meant to be used with integration and e2e tests.
// It runs an catch-all http server that stores submitted payloads into a dictionary of [api.Payloads], indexed by the route
// It implements 3 testing endpoints:
//   - /fakeintake/payloads/<payload_route> returns any received payloads on the specified route as [api.Payload]s
//   - /fakeintake/health returns current fakeintake server health
//   - /fakeintake/routestats returns stats for collected payloads, by route
//   - /fakeintake/flushPayloads returns all stored payloads and clear them up
//
// [api.Payloads]: https://pkg.go.dev/github.com/DataDog/datadog-agent@main/test/fakeintake/api#Payload
package server

import (
	"net/http"
	"strings"
)

func redactHeader(header http.Header) http.Header {
	if header == nil {
		return header
	}
	safeHeader := make(http.Header, len(header))
	for key, values := range header {
		if !strings.Contains(strings.ToLower(key), "key") {
			for _, value := range values {
				safeHeader.Add(key, value)
			}
			continue
		}
		safeHeader.Add(strings.ToLower(key), "<redacted>")
	}
	return safeHeader
}
