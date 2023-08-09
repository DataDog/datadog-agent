// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package logs TODO comment
package logs

import (
	"io"
	"net/http"
)

// LambdaLogsAPIServer implements the AWS Lambda Logs API callback
type LambdaLogsAPIServer struct {
	out chan<- []LambdaLogAPIMessage
}

// NewLambdaLogsAPIServer exported function should have comment or be unexported
func NewLambdaLogsAPIServer(out chan<- []LambdaLogAPIMessage) LambdaLogsAPIServer {
	return LambdaLogsAPIServer{out}
}

// Close exported method should have comment or be unexported
func (l *LambdaLogsAPIServer) Close() {
	close(l.out)
}

// ServeHTTP - see type LambdaLogsCollector comment.
func (l *LambdaLogsAPIServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	data, _ := io.ReadAll(r.Body)
	defer r.Body.Close()
	messages, err := parseLogsAPIPayload(data)
	if err != nil {
		w.WriteHeader(400)
	} else {
		l.out <- messages
		w.WriteHeader(200)
	}
}
