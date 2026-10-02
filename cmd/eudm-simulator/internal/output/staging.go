// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package output

import (
	"net/http"
	"net/url"

	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/safety"
)

// stagingTransport checks every attempt, including requests produced by HTTP
// redirects. The underlying transport is the Agent's normal HTTP transport.
type stagingTransport struct{ base http.RoundTripper }

func (t stagingTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	origin := url.URL{Scheme: request.URL.Scheme, Host: request.URL.Host, User: request.URL.User}
	if err := safety.ValidateEndpoint(origin.String()); err != nil {
		return nil, err
	}
	return t.base.RoundTrip(request)
}
