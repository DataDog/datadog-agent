// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"encoding/json"
	"net/http"
)

type httpResponse struct {
	contentType string
	statusCode  int
	body        []byte
}

func writeHTTPResponse(w http.ResponseWriter, response httpResponse) {
	if response.contentType != "" {
		w.Header().Set("Content-Type", response.contentType)
	}
	w.WriteHeader(response.statusCode)
	if len(response.body) > 0 {
		w.Write(response.body)
	}
}

func buildErrorResponse(responseError error) httpResponse {
	resp := errorResponseBody{Errors: []string{responseError.Error()}}
	return buildResponse(resp, http.StatusBadRequest, "application/json")
}

func buildSuccessResponse(body interface{}) httpResponse {
	return buildResponse(body, http.StatusOK, "application/json")
}

func buildResponse(body interface{}, statusCode int, contentType string) httpResponse {
	resp := httpResponse{contentType: contentType, statusCode: statusCode}

	bodyJSON, err := json.Marshal(body)

	if err != nil {
		return httpResponse{
			statusCode:  http.StatusInternalServerError,
			contentType: "text/plain",
			body:        []byte(err.Error()),
		}
	}

	resp.body = bodyJSON
	return resp
}
