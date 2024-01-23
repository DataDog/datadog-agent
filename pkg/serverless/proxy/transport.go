// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package proxy

import (
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/serverless/invocationlifecycle"
)

type proxyTransport struct {
	processor invocationlifecycle.InvocationProcessor
}

func (p *proxyTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	panic("not called")
}

func processResponse(p *proxyTransport, request *http.Request, response *http.Response) error {
	panic("not called")
}

func processRequest(p *proxyTransport, request *http.Request) error {
	panic("not called")
}
