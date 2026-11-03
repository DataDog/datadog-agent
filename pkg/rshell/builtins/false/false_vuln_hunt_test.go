// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Vuln-hunt campaign: 2026-05-19-codex.
package falsecmd

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

type panicReader struct{}

func (panicReader) Read([]byte) (int, error) {
	panic("false must not read stdin")
}

type panicWriter struct {
	name string
}

func (w panicWriter) Write([]byte) (int, error) {
	panic("false must not write " + w.name)
}

func TestVulnHuntBuiltinResourceExhaustion_InertFailureInputs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	callCtx := &builtins.CallContext{
		Stdin:  panicReader{},
		Stdout: panicWriter{name: "stdout"},
		Stderr: panicWriter{name: "stderr"},
		OpenFile: func(context.Context, string, int, os.FileMode) (io.ReadWriteCloser, error) {
			t.Fatal("false must not open files")
			return nil, nil
		},
	}

	hugeNumericOperand := strings.Repeat("9", 64*1024)
	got := run(ctx, callCtx, []string{"--help", "--unknown", "--", hugeNumericOperand})
	want := builtins.Result{Code: 1}
	if got != want {
		t.Fatalf("false returned %+v, want %+v", got, want)
	}
}
