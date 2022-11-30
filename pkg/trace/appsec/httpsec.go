// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package appsec

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/trace/api"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	waf "github.com/DataDog/go-libddwaf"
	"github.com/pkg/errors"
)

type (
	requestPayload struct {
		Trace   bool    `json:"trace"`
		Request Request `json:"request"`
		Service string  `json:"service"`
	}

	Request struct {
		Method     string   `json:"method"`
		URL        *url.URL `json:"url"`
		RemoteAddr string   `json:"remote_addr"`
		// Headers normalized with lower-case names.
		Headers map[string]string `json:"headers"`
	}

	responsePayload struct {
		Type    string   `json:"type"`
		Matches []byte   `json:"matches"`
		Actions []string `json:"actions"`
	}

	errorPayload struct {
		Type  string `json:"type"`
		Error string `json:"error"`
	}
)

func NewHTTPSecHandler(handle *waf.Handle, traceChan chan *api.Payload) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Parse the body
		var reqPayload requestPayload
		body := r.Body
		defer func() {
			io.ReadAll(body)
			body.Close()
		}()
		if err := json.NewDecoder(body).Decode(&reqPayload); err != nil {
			writeErrorResponse(w, errors.Wrap(err, "appsec: couldn't parse the request body into json:"))
			return
		}

		request := reqPayload.Request
		headers := request.Headers
		traceID, parentID := getSpanContext(headers)
		sp := startHTTPRequestSpan(traceID, parentID, reqPayload.Service, request.RemoteAddr, request.Method, request.URL, headers)

		defer func() {
			sp.finish()
			sendSpan(sp.Span, int32(sampler.PriorityUserKeep), traceChan)
		}()

		wafCtx := waf.NewContext(handle)
		if wafCtx == nil {
			// The WAF handle got released in the meantime
			writeUnavailableResponse(w)
			return
		}
		defer wafCtx.Close()

		addresses := makeHTTPSecAddresses(reqPayload.Request, sp.Meta["http.client_ip"])
		log.Debug("appsec: httpsec api: running the security rules against %v", addresses)
		matches, actions, err := wafCtx.Run(addresses, defaultWAFTimeout)
		if err != nil && err != waf.ErrTimeout {
			writeErrorResponse(w, err)
			return
		}
		log.Debug("appsec: httpsec api: matches=%s actions=%v", string(matches), actions)

		if len(matches) > 0 {
			setSecurityEventsTags(sp, matches, reqPayload.Request.Headers, nil)
		}
		if len(actions) > 0 {
			sp.Meta["blocked"] = "true"
		}

		if err := json.NewEncoder(w).Encode(responsePayload{
			Type:    "waf_response",
			Matches: matches,
			Actions: actions,
		}); err != nil {
			log.Errorf("appsec: unexpected error while encoding the waf response payload into json: %v", err)
		}
	})
}

func makeHTTPSecAddresses(req Request, clientIP string) map[string]interface{} {
	headers := map[string]string{}
	for h, v := range req.Headers {
		h = strings.ToLower(h)
		if h == "cookie" {
			continue
		}
		headers[h] = v
	}
	addr := map[string]interface{}{
		"server.request.headers.no_cookies": headers,
		"server.request.uri.raw":            req.URL.RequestURI(),
		"server.request.query":              req.URL.Query(),
	}
	if clientIP != "" {
		addr["http.client_ip"] = clientIP
	}
	return addr
}

func writeErrorResponse(w http.ResponseWriter, err error) {
	w.WriteHeader(http.StatusInternalServerError)
	log.Error(err)
	res := errorPayload{
		Type:  "error",
		Error: err.Error(),
	}
	if err := json.NewEncoder(w).Encode(res); err != nil {
		log.Errorf("appsec: couldn't encode the error response payload `%q` into json: %v", res, err)
	}
}

func writeUnavailableResponse(w http.ResponseWriter) {
	w.WriteHeader(http.StatusServiceUnavailable)
}
