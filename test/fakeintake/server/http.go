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
	data        interface{}
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
	return updateResponseFromData(httpResponse{
		statusCode:  http.StatusBadRequest,
		contentType: "application/json",
		data:        errorResponseBody{Errors: []string{responseError.Error()}},
	})
}

func updateResponseFromData(r httpResponse) httpResponse {
	if r.contentType == "application/json" {
		bodyJSON, err := json.Marshal(r.data)
		if err != nil {
			return httpResponse{
				statusCode:  http.StatusInternalServerError,
				contentType: "text/plain",
				body:        []byte(err.Error()),
			}
		}
		r.body = bodyJSON
	} else {
		r.body = r.data.([]byte)
	}
	return r
}
