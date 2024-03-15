// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(SERV) Fix revive linter
package logs

import (
	"io"
	"net/http"
)

// LambdaLogsAPI implements the AWS Lambda Logs API callback
//
//nolint:revive // TODO(SERV) Fix revive linter
type LambdaLogsAPIServer struct {
	out chan<- []LambdaLogAPIMessage
}

//nolint:revive // TODO(SERV) Fix revive linter
func NewLambdaLogsAPIServer(out chan<- []LambdaLogAPIMessage) LambdaLogsAPIServer {
	return LambdaLogsAPIServer{out}
}

//nolint:revive // TODO(SERV) Fix revive linter
func (l *LambdaLogsAPIServer) Close() {
	close(l.out)
}

// ServeHTTP - see type LambdaLogsCollector comment.
//
//nolint:revive // TODO(SERV) Fix revive linter
func (c *LambdaLogsAPIServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	data, _ := io.ReadAll(r.Body)
	defer r.Body.Close()
	messages, err := parseLogsAPIPayload(data)
	if err != nil {
		w.WriteHeader(400)
	} else {
		c.out <- messages
		w.WriteHeader(200)
	}
}
