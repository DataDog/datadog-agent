// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package sort

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
	"github.com/stretchr/testify/assert"
)

func TestCheckSortedRespectsContextCancellation(t *testing.T) {
	// A cancelled context should be detected on the first iteration,
	// even for small inputs.
	lines := []string{"a", "b", "c"}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	var stderr bytes.Buffer
	callCtx := &builtins.CallContext{
		Stdout: &bytes.Buffer{},
		Stderr: &stderr,
	}

	cmpFn := func(a, b string) int {
		return strings.Compare(a, b)
	}

	result := checkSorted(ctx, callCtx, lines, cmpFn, false, false, "-")

	assert.Equal(t, uint8(1), result.Code,
		"checkSorted should return exit code 1 when context is cancelled")
}

func TestCheckSortedCompletesWithoutCancellation(t *testing.T) {
	// Verify that checkSorted works normally without cancellation.
	lines := []string{"a", "b", "c"}

	var stderr bytes.Buffer
	callCtx := &builtins.CallContext{
		Stdout: &bytes.Buffer{},
		Stderr: &stderr,
	}

	cmpFn := func(a, b string) int {
		return strings.Compare(a, b)
	}

	result := checkSorted(context.Background(), callCtx, lines, cmpFn, false, false, "-")

	assert.Equal(t, uint8(0), result.Code,
		"checkSorted should return exit code 0 for sorted input")
}

func TestCheckSortedDetectsDisorder(t *testing.T) {
	lines := []string{"b", "a", "c"}

	var stderr bytes.Buffer
	callCtx := &builtins.CallContext{
		Stdout: &bytes.Buffer{},
		Stderr: &stderr,
	}

	cmpFn := func(a, b string) int {
		return strings.Compare(a, b)
	}

	result := checkSorted(context.Background(), callCtx, lines, cmpFn, false, false, "-")

	assert.Equal(t, uint8(1), result.Code)
	assert.Contains(t, stderr.String(), "disorder")
}
