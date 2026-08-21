// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"context"
	"fmt"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"
)

//nolint:all
//go:noinline
func stackA() {
	stackB()
}

//nolint:all
//go:noinline
func stackB() {
	stackC()
}

//nolint:all
//go:noinline
func stackC() string {
	return fmt.Sprintf("hello %d!", 1)
}

//nolint:all
func executeStack(ctx context.Context) {
	span, _ := tracer.StartSpanFromContext(ctx, "sample.stack")
	defer span.Finish()

	stackA()
}
