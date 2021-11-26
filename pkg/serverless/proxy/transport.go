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

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type proxyTransport struct {
	currentInvocationDetails *invocationDetails
	processor                invocationProcessor
}

func (p *proxyTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	log.Debug("[proxy] new request to %s", request.URL)

	// enrich the currentInvocationDetails object
	requestProcessor(p.currentInvocationDetails, request)
	if p.currentInvocationDetails.isComplete() {
		p.processor.process(p.currentInvocationDetails)
	}

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

	// enrich the currentInvicationDetails when /next response is received
	if request.Method == "GET" && strings.HasSuffix(request.URL.String(), "/next") {
		p.currentInvocationDetails.startTime = time.Now()
		p.currentInvocationDetails.invokeHeaders = response.Header
		p.currentInvocationDetails.invokeEventPayload = string(dumpedResponse[indexPayload:])
	}

	return response, nil
}

func requestProcessor(invocationDetails *invocationDetails, request *http.Request) {
	if request.Method == "GET" && strings.HasSuffix(request.URL.String(), "/next") {
		invocationDetails.reset()
	} else if request.Method == "POST" && strings.HasSuffix(request.URL.String(), "/response") {
		invocationDetails.endTime = time.Now()
		invocationDetails.isError = false
	} else if request.Method == "POST" && strings.HasSuffix(request.URL.String(), "/error") {
		invocationDetails.endTime = time.Now()
		invocationDetails.isError = true
		// TODO create and send the error enhanced metric here
	} else {
		log.Debug("[proxy] unknown verb/url (%s/%s) pattern found, ignoring", request.Method, request.URL.String())
	}
}
