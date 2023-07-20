// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

type errorResponseBody struct {
	Errors []string `json:"errors"`
}

// Same struct as in datadog-agent/comp/core/flare/helpers/send_flare.go
type flareResponseBody struct {
	CaseID int    `json:"case_id,omitempty"`
	Error  string `json:"error,omitempty"`
}

// getResponseBodyFromURLPath returns the appropriate response body to HTTP request sent to 'urlPath'
func getResponseBodyFromURLPath(urlPath string) interface{} {
	var body interface{}

	if urlPath == "/support/flare" {
		body = flareResponseBody{}
	} else {
		body = errorResponseBody{}
	}

	return body
}
