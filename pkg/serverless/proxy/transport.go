// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package proxy

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/serverless/invocationlifecycle"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type proxyTransport struct {
	processor invocationlifecycle.InvocationProcessor
}

func (p *proxyTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	log.Debug("[proxy] new request to %s", request.URL)

	processRequest(p, request)

	response, err := http.DefaultTransport.RoundTrip(request)
	if err != nil {
		log.Error("could not forward the request", err)
		return nil, err
	}

	dumpedResponse, err := httputil.DumpResponse(response, true)
	if err != nil {
		log.Error("could not dump the response")
		return nil, err
	}

	// extract only the payload as headers can be retrieved without inspecting the response
	indexPayload := bytes.Index(dumpedResponse, []byte("\r\n\r\n"))
	if indexPayload == -1 {
		return nil, errors.New("invalid payload format")
	}

	// triggers onInvokeStart when /next response is received
	if request.Method == "GET" && strings.HasSuffix(request.URL.String(), "/next") {
		details := &invocationlifecycle.InvocationStartDetails{
			StartTime:             time.Now(),
			InvokeEventRawPayload: dumpedResponse[indexPayload:],
		}
		p.processor.OnInvokeStart(details)
	}

	return response, nil
}

func processRequest(p *proxyTransport, request *http.Request) {
	if request.Method == "POST" && strings.HasSuffix(request.URL.String(), "/response") {
		details := &invocationlifecycle.InvocationEndDetails{
			EndTime: time.Now(),
			IsError: false,
		}
		p.processor.OnInvokeEnd(details)
	} else if request.Method == "POST" && strings.HasSuffix(request.URL.String(), "/error") {
		details := &invocationlifecycle.InvocationEndDetails{
			EndTime: time.Now(),
			IsError: true,
		}
		p.processor.OnInvokeEnd(details)
	} else {
		log.Debug("[proxy] unknown verb/url (%s/%s) pattern found, ignoring", request.Method, request.URL.String())
	}
}
