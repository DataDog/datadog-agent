// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package appsec

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/trace/log"
	waf "github.com/DataDog/go-libddwaf"
	"github.com/pkg/errors"
)

type (
	requestPayload struct {
		Type         string                 `json:"type"`
		Trace        bool                   `json:"trace"`
		ExtraTags    spanTags               `json:"extra_tags"`
		SecAddresses map[string]interface{} `json:"sec_addresses"`
		TraceID      uint64                 `json:"trace_id"`
		ParentID     uint64                 `json:"parent_id"`
		Resource     string                 `json:"resource"`
	}

	spanTags struct {
		Meta    map[string]string  `json:"meta"`
		Metrics map[string]float64 `json:"metrics"`
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

func NewHTTPSecHandler(handle *waf.Handle) http.Handler {
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

		sp := startHTTPRequestSpan(reqPayload.TraceID, reqPayload.ParentID, reqPayload.Resource)
		sp.Meta = reqPayload.ExtraTags.Meta
		sp.Metrics = reqPayload.ExtraTags.Metrics
		defer func() {
			sp.finish()
			// TODO: add the span into the trace queue of the trace agent
		}()

		wafCtx := waf.NewContext(handle)
		if wafCtx == nil {
			// The WAF handle got released in the meantime
			writeUnavailableResponse(w)
			return
		}
		defer wafCtx.Close()

		log.Debug("appsec: httpsec api: running the security rules against %v", reqPayload.SecAddresses)
		matches, actions, err := wafCtx.Run(reqPayload.SecAddresses, defaultWAFTimeout)
		if err != nil && err != waf.ErrTimeout {
			writeErrorResponse(w, err)
			return
		}
		log.Debug("appsec: httpsec api: matches=%s actions=%v", string(matches), actions)

		if err := json.NewEncoder(w).Encode(responsePayload{
			Type:    "waf_response",
			Matches: matches,
			Actions: actions,
		}); err != nil {
			log.Errorf("appsec: unexpected error while encoding the waf response payload into json: %v", err)
		}
	})
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
