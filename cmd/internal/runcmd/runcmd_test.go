// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package runcmd

import (
	"bytes"
	"errors"
	"os"
	"regexp"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
)

func TestRun_success(t *testing.T) {
	cmd := &cobra.Command{
		Use: "ok",
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
	cmd.SetArgs([]string{"ok"})
	require.Equal(t, 0, Run(cmd))
}

func TestRun_fail(t *testing.T) {
	cmd := &cobra.Command{
		Use: "bad",
		RunE: func(cmd *cobra.Command, args []string) error {
			return errors.New("uhoh")
		},
	}
	cmd.SetArgs([]string{"bad"})
	require.Equal(t, -1, Run(cmd))
}

func makeFxError(t *testing.T) error { //nolint:revive // TODO fix revive unused-parameter
	app := fx.New(
		fx.Provide(func() (string, error) {
			return "", errors.New("uhoh")
		}),
		fx.Invoke(func(s string) {}),
	)

	// "could not build arguments for function .. uhoh"
	return app.Err()
}

func TestDisplayError_normalError(t *testing.T) {
	var buf bytes.Buffer
	displayError(errors.New("uhoh"), &buf)
	require.Equal(t, "Error: uhoh\n", buf.String())
}

// fx errors are abbreviated to just the root cause by default
func TestDisplayError_fxError(t *testing.T) {
	var buf bytes.Buffer
	t.Setenv("TRACE_FX", "") // get testing to reset this value for us
	os.Unsetenv("TRACE_FX")  // but actually _unset_ the value
	displayError(makeFxError(t), &buf)
	require.Equal(t, "Error: uhoh\n", buf.String())
}

// entire error is included with TRACE_FX set
func TestDisplayError_fxError_TRACE_FX(t *testing.T) {
	var buf bytes.Buffer
	t.Setenv("TRACE_FX", "1")
	displayError(makeFxError(t), &buf)
	require.Regexp(t, regexp.MustCompile("Error: could not build arguments for function .* uhoh"), buf.String())
}
