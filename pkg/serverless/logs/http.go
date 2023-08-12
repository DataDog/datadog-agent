// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logs

import (
	"io"
	"net/http"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/valyala/fasthttp"
)

// LambdaLogsAPI implements the AWS Lambda Logs API callback
type LambdaLogsAPIServer struct {
	out chan<- []LambdaLogAPIMessage
}

func NewLambdaLogsAPIServer(out chan<- []LambdaLogAPIMessage) LambdaLogsAPIServer {
	return LambdaLogsAPIServer{out}
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
		c.out <- messages
		w.WriteHeader(200)
	}
}

// FastRouter - fast
func (c *LambdaLogsAPIServer) FastRouter(ctx *fasthttp.RequestCtx) {
	switch string(ctx.Path()) {
	// case "/lambda/logs":
	// 	messages, _ := parseLogsAPIPayload(ctx.PostBody())
	// 	c.out <- messages
	default:
		body := string(ctx.PostBody())
		if strings.Contains(body, "logsDropped") {
			log.Error("Log dropped")
		}
		// messages, _ := parseLogsAPIPayload(ctx.PostBody())
		// for _, message := range messages {
		// 	if strings.Contains(message.stringRecord, "logsDropped") {
		// 		log.Error("Log dropped")
		// 	}
		// }
		// c.out <- messages
	}
}
