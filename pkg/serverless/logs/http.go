// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logs

import (
	"io"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/serverless/logsyncorchestrator"
)

// LambdaLogsAPI implements the AWS Lambda Logs API callback
type LambdaLogsAPIServer struct {
	out                 chan<- []LambdaLogAPIMessage
	LogSyncOrchestrator *logsyncorchestrator.LogSyncOrchestrator
}

func NewLambdaLogsAPIServer(out chan<- []LambdaLogAPIMessage, logSyncOrchestrator *logsyncorchestrator.LogSyncOrchestrator) LambdaLogsAPIServer {
	return LambdaLogsAPIServer{
		out:                 out,
		LogSyncOrchestrator: logSyncOrchestrator,
	}
}

func (l *LambdaLogsAPIServer) Close() {
	close(l.out)
}

// ServeHTTP - see type LambdaLogsCollector comment.
func (c *LambdaLogsAPIServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	data, _ := io.ReadAll(r.Body)
	defer r.Body.Close()
	messages, err := parseLogsAPIPayload(data)
	if err != nil {
		w.WriteHeader(400)
	} else {
		go func() {
			c.LogSyncOrchestrator.WaitIncomingRequest()
			c.out <- messages
		}()
		w.WriteHeader(200)
	}
}
