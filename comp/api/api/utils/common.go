// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package utils has common utility methods that components can use for structuring http responses of their endpoints
package utils

import (
	"encoding/json"
	"net"
	"net/http"

	grpccontext "github.com/DataDog/datadog-agent/pkg/util/grpc/context"
)

// SetJSONError writes a server error as JSON with the correct http error code
func SetJSONError(w http.ResponseWriter, err error, errorCode int) {
	w.Header().Set("Content-Type", "application/json")
	body, _ := json.Marshal(map[string]string{"error": err.Error()})
	http.Error(w, string(body), errorCode)
}

// GetConnection returns the connection for the request
func GetConnection(r *http.Request) net.Conn {
	return r.Context().Value(grpccontext.ConnContextKey).(net.Conn)
}
