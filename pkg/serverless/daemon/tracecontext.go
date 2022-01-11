// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package daemon

import (
	"fmt"
	"math/rand"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// TraceContext is a route called by tracer so they can retrieve the tracing context
type TraceContext struct {
}

// TraceContext - see type TraceContext comment.
func (tc *TraceContext) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Debug("Hit the serverless.TraceContext route.")

	// TODO use traceID and spanID from the generated span
	traceID := uint64(rand.Uint32())<<32 + uint64(rand.Uint32())
	spanID := uint64(rand.Uint32())<<32 + uint64(rand.Uint32())

	w.Header().Set("x-datadog-trace-id", fmt.Sprintf("%v", traceID))
	w.Header().Set("x-datadog-span-id", fmt.Sprintf("%v", spanID))
	w.WriteHeader(200)
}
