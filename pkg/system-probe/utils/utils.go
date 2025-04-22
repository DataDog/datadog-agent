// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"encoding/json"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// WriteAsJSON marshals the give data argument into JSON and writes it to the `http.ResponseWriter`
func WriteAsJSON(w http.ResponseWriter, data interface{}) {
	buf, err := json.Marshal(data)
	if err != nil {
		log.Errorf("unable to marshal data into JSON: %s", err)
		w.WriteHeader(500)
		return
	}
	_, _ = w.Write(buf)
}

// GetClientID gets client provided in the http request, defaulting to -1
func GetClientID(req *http.Request) string {
	var clientID = "-1"
	if rawCID := req.URL.Query().Get("client_id"); rawCID != "" {
		clientID = rawCID
	}
	return clientID
}
