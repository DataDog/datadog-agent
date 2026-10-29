// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !linux

package main

import (
	"context"
	"fmt"
	"io"
)

func runPrivilegedHelper(_ context.Context, _ []string, stderr io.Writer) int {
	fmt.Fprintln(stderr, "privileged-helper is supported only on Linux")
	return 1
}

func runPrivilegedWorker(_ context.Context, _ []string, _ io.Reader, _ io.Writer, stderr io.Writer) int {
	fmt.Fprintln(stderr, "privileged-worker is supported only on Linux")
	return 1
}
