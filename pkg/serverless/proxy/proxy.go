// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(SERV) Fix revive linter
package proxy

import (
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/DataDog/datadog-agent/pkg/serverless/invocationlifecycle"
)

type runtimeProxy struct {
	target    *url.URL
	proxy     *httputil.ReverseProxy
	processor invocationlifecycle.InvocationProcessor
}

// Start starts the proxy
// This proxy allows us to inspect traffic from/to the AWS Lambda Runtime API
func Start(proxyHostPort string, originalRuntimeHostPort string, processor invocationlifecycle.InvocationProcessor) {
	panic("not called")
}

func setup(proxyHostPort string, originalRuntimeHostPort string, processor invocationlifecycle.InvocationProcessor) {
	panic("not called")
}

func (rp *runtimeProxy) handle(w http.ResponseWriter, r *http.Request) {
	panic("not called")
}

func newProxy(target string, processor invocationlifecycle.InvocationProcessor) *runtimeProxy {
	panic("not called")
}
