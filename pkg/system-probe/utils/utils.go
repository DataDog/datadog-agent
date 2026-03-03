// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// FormatOptions represents formatting options for WriteAsJson.
type FormatOptions bool

const (
	// CompactOutput indicates that the output should be compact (no indentation).
	CompactOutput = false
	// PrettyPrint indicates that the output should be pretty-printed (with indentation).
	PrettyPrint = true
)

func isTruthy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "t", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

const (
	// PrettyPrintQueryParam is the query parameter used to request pretty-printed JSON output
	PrettyPrintQueryParam = "pretty_print"
)

// GetPrettyPrintFromQueryParams returns true if the pretty_print query parameter is set to "true" in the request URL
func GetPrettyPrintFromQueryParams(req *http.Request) FormatOptions {
	if prettyPrint := req.URL.Query().Get(PrettyPrintQueryParam); isTruthy(prettyPrint) {
		return PrettyPrint
	}
	return CompactOutput
}

// WriteAsJSON marshals the give data argument into JSON and writes it to the `http.ResponseWriter`
func WriteAsJSON(w http.ResponseWriter, data interface{}, outputOptions FormatOptions) {
	encoder := json.NewEncoder(w)
	//nolint:staticcheck // S1002: explicit comparison preferred for readability
	if outputOptions == PrettyPrint {
		encoder.SetIndent("", "  ")
	}
	err := encoder.Encode(data)
	if err != nil {
		log.Errorf("unable to marshal data into JSON: %s", err)
		w.WriteHeader(500)
		return
	}
}

// GetClientID gets client provided in the http request, defaulting to -1
func GetClientID(req *http.Request) string {
	var clientID = "-1"
	if rawCID := req.URL.Query().Get("client_id"); rawCID != "" {
		clientID = rawCID
	}
	return clientID
}
