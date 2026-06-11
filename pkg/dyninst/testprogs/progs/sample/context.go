// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"context"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"
)

// testContextSpanID is the deterministic span (and, with 128-bit trace-id
// generation disabled in main.go, trace) id we plant on the active dd-trace
// span before invoking testTakeContext. The chase walk extracts these from
// the captured ctx; the decoded snapshot's dd.trace_id and field-site
// trace_id should both equal 0123456789abcdef.
const testContextSpanID = uint64(0x0123456789abcdef)

//go:noinline
func testTakeContext(ctx context.Context) {
	_ = ctx
}

func executeContextFuncs() {
	span, ctx := tracer.StartSpanFromContext(
		context.Background(), "sample.context.test",
		tracer.WithSpanID(testContextSpanID),
	)
	defer span.Finish()
	testTakeContext(ctx)
}
