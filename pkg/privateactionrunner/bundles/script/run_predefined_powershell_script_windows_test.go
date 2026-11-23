// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

package com_datadoghq_script

import (
	"bytes"
	"context"
	"encoding/base64"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestPowershellInlineScriptParameterRoundTrip runs the inline-script path
// through a real powershell.exe and checks that each parameter value reaches
// the script byte-for-byte. The script prints the value as base64 so the
// result does not depend on the console output code page.
func TestPowershellInlineScriptParameterRoundTrip(t *testing.T) {
	if _, err := exec.LookPath("powershell.exe"); err != nil {
		t.Skip("powershell.exe not available")
	}

	values := []string{
		"plain",
		"d'Artagnan",
		"\u2018",
		"\u2019",
		"\u201A",
		"\u201B",
		"it\u2019s",
		"\u2018quoted\u2019",
		"\u201Alow\u201B",
		"'\u2019",
		"\u2019'",
		"\u2018\u2019\u201A\u201B'",
		"a\u2019b'c\u2018d; e",
		"$env:TEMP `n \"double\"",
		"\u201Cdouble\u201D \u201E\u201F",
		"line1\nline2\u2019s",
	}

	config := RunPredefinedPowershellScriptConfig{
		Script: "[Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes({{ parameters.value }}))",
	}
	for _, value := range values {
		t.Run(value, func(t *testing.T) {
			evaluated, err := evaluatePowershellScript(config, map[string]any{"value": value})
			require.NoError(t, err)

			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			cmd := newPowershellCommand(ctx, evaluated, nil)
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			require.NoError(t, cmd.Run(), "stderr: %s", stderr.String())
			require.Equal(t,
				base64.StdEncoding.EncodeToString([]byte(value)),
				strings.TrimSpace(stdout.String()),
			)
		})
	}
}
