// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package proxy

import (
	"bytes"
	"context"
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
	log.Debugf("runtime api proxy: new request to %s", request.URL)

	if err := processRequest(p, request); err != nil {
		log.Error("runtime api proxy: error while processing the request:", err)
	}

	response, err := http.DefaultTransport.RoundTrip(request)
	if err != nil {
		if err == context.Canceled {
			log.Debug("runtime api proxy: context cancelled:", request.Context().Err())
		} else {
			log.Error("runtime api proxy: could not forward the request", err)
		}
		return nil, err
	}

	if err := processResponse(p, request, response); err != nil {
		log.Error("runtime api proxy: error while processing the response:", err)
	}

	return response, nil
}

func processResponse(p *proxyTransport, request *http.Request, response *http.Response) error {
	dumpedResponse, err := httputil.DumpResponse(response, true)
	if err != nil {
		return err
	}

	// triggers onInvokeStart when /next response is received
	switch {
	case request.Method == "GET" && strings.HasSuffix(request.URL.String(), "/next"):
		// extract only the payload as headers can be retrieved without inspecting the response
		indexPayload := bytes.Index(dumpedResponse, []byte("\r\n\r\n"))
		if indexPayload == -1 {
			return errors.New("invalid payload format")
		}
		payload := dumpedResponse[indexPayload:]
		log.Debugf("runtime api proxy: /next: processing event payload `%s`", payload)
		details := &invocationlifecycle.InvocationStartDetails{
			StartTime:             time.Now(),
			InvokeEventRawPayload: payload,
		}
		p.processor.OnInvokeStart(details)
	}

	return nil
}

func processRequest(p *proxyTransport, request *http.Request) error {
	body, err := httputil.DumpRequest(request, true)
	if err != nil {
		log.Error("could not dump the request:", err)
		return err
	}
	indexPayload := bytes.Index(body, []byte("\r\n\r\n"))
	if indexPayload == -1 {
		return errors.New("invalid request payload format")
	}
	body = body[indexPayload:]

	switch {
	case request.Method == "POST" && strings.HasSuffix(request.URL.String(), "/response"):
		log.Debugf("runtime api proxy: /response: processing response payload `%s`", body)
		details := &invocationlifecycle.InvocationEndDetails{
			EndTime:            time.Now(),
			IsError:            false,
			ResponseRawPayload: body,
		}
		p.processor.OnInvokeEnd(details)

	case request.Method == "POST" && strings.HasSuffix(request.URL.String(), "/error"):
		log.Debugf("runtime api proxy: /error: processing response payload `%s`", body)
		details := &invocationlifecycle.InvocationEndDetails{
			EndTime:            time.Now(),
			IsError:            true,
			ResponseRawPayload: body,
		}
		p.processor.OnInvokeEnd(details)

	default:
		log.Debugf("runtime api proxy: ignoring %s /%s", request.Method, request.URL.String())
	}

	return nil
}
