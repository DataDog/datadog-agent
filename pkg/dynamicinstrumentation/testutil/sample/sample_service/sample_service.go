// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Sample-service is a simple program with lots of functions to test GoDI against
package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"time"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	defer func() {
		log.Println("Stopping sample process")
	}()

	tracerEnabled := os.Getenv("DD_SERVICE") != "" &&
		os.Getenv("DD_DYNAMIC_INSTRUMENTATION_ENABLED") != "" &&
		os.Getenv("DD_DYNAMIC_INSTRUMENTATION_OFFLINE") == ""

	log.Println("Starting sample process with tracerEnabled=", tracerEnabled)

	if tracerEnabled {
		ddAgentHost := os.Getenv("DD_AGENT_HOST")
		if ddAgentHost == "" {
			ddAgentHost = "localhost"
		}
		// Start the tracer and defer the Stop method.
		err := tracer.Start(tracer.WithAgentAddr(net.JoinHostPort(ddAgentHost, "8126")),
			tracer.WithDebugMode(true))
		if err != nil {
			log.Printf("error starting tracer: %s", err)
		}
		defer tracer.Stop()
	}

	ticker := time.NewTicker(time.Second / 2)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			stop()
			return
		case <-ticker.C:
			sample.ExecuteOther()
			sample.ExecuteBasicFuncs()
			sample.ExecuteMultiParamFuncs()
			sample.ExecuteStringFuncs()
			sample.ExecuteArrayFuncs()
			sample.ExecuteSliceFuncs()
			sample.ExecuteStructFuncs()
			sample.ExecuteStackAndInlining()
			sample.ExecutePointerFuncs()
			sample.ExecuteComplexFuncs()

			// unsupported for MVP, should not cause crashes
			sample.ExecuteGenericFuncs()
			sample.ExecuteMapFuncs()
			sample.ExecuteInterfaceFuncs()
			go sample.Return_goroutine_id()
		}
	}
}
