// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import "net/http"

type httpResponse struct {
	contentType string
	statusCode  int
	body        []byte
}

func writeHttpResponse(w http.ResponseWriter, response httpResponse) {
	if response.contentType != "" {
		w.Header().Set("Content-Type", response.contentType)
	}
	w.WriteHeader(response.statusCode)
	if len(response.body) > 0 {
		w.Write(response.body)
	}
}
