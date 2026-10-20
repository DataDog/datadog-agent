// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build unix

package tr_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mvdan.cc/sh/v3/syntax"

	"github.com/DataDog/datadog-agent/pkg/rshell/internal/interpoption"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// H8: an explicitly allowed infinite source must stop once the runner context
// is canceled. This uses io.Discard so the test measures cancellation rather
// than stdout buffering.
func TestVulnHuntBuiltinSpecialFiles_DevZeroStopsOnContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	prog, err := syntax.NewParser().Parse(strings.NewReader("tr a b < /dev/zero"), "")
	require.NoError(t, err)

	var stderr bytes.Buffer
	runner, err := interp.New(
		interp.StdIO(nil, io.Discard, &stderr),
		interpoption.AllowAllCommands().(interp.RunnerOption),
		interp.AllowedPaths([]string{"/dev"}),
	)
	require.NoError(t, err)
	defer runner.Close()

	err = runner.Run(ctx, prog)
	assert.True(t, errors.Is(err, context.DeadlineExceeded), "got %v", err)
	assert.Empty(t, stderr.String())
}
