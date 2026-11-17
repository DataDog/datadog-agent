// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package printf_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mvdan.cc/sh/v3/syntax"

	"github.com/DataDog/datadog-agent/pkg/rshell/internal/interpoption"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// Campaign: vuln-hunt/2026-05-19-codex. These tests pin adversarial
// printf surfaces that should remain public-safe blocked-attack regressions.

func TestVulnHuntBuiltinResourceExhaustion_LiteralWidthOverflowDoesNotAllocate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	format := "%" + strings.Repeat("9", 2048) + "s\\n"
	stdout, _, code := runScriptCtx(ctx, t, "printf '"+format+"' x", "")

	assert.Equal(t, 0, code)
	assert.Less(t, len(stdout), 1024)
}

func TestVulnHuntBuiltinResourceExhaustion_LiteralPrecisionOverflowDoesNotAllocate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	format := "%." + strings.Repeat("9", 2048) + "f\\n"
	stdout, _, code := runScriptCtx(ctx, t, "printf '"+format+"' 3.14", "")

	assert.Equal(t, 0, code)
	assert.Less(t, len(stdout), 1024)
}

func TestVulnHuntBuiltinResourceExhaustion_OutputLimitStopsLargeFormatReuse(t *testing.T) {
	parser := syntax.NewParser()
	prog, err := parser.Parse(strings.NewReader(`printf "%10000s\n"`+strings.Repeat(" x", 1200)), "")
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer
	runner, err := interp.New(
		interp.StdIO(nil, &stdout, &stderr),
		interpoption.AllowAllCommands().(interp.RunnerOption),
	)
	require.NoError(t, err)
	defer runner.Close()

	err = runner.Run(context.Background(), prog)
	require.Error(t, err)
	assert.True(t, errors.Is(err, interp.ErrOutputLimitExceeded), "got %v", err)
	assert.LessOrEqual(t, stdout.Len(), 10*1024*1024)
	assert.Empty(t, stderr.String())
}
