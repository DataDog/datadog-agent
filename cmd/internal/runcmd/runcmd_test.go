// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package runcmd

import (
	"bytes"
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
)

func TestRun_success(t *testing.T) {
	cmd := &cobra.Command{
		Use: "ok",
		RunE: func(_ *cobra.Command, _ []string) error {
			return nil
		},
	}
	cmd.SetArgs([]string{"ok"})
	require.Equal(t, 0, Run(cmd))
}

func TestRun_fail(t *testing.T) {
	cmd := &cobra.Command{
		Use: "bad",
		RunE: func(_ *cobra.Command, _ []string) error {
			return errors.New("uhoh")
		},
	}
	cmd.SetArgs([]string{"bad"})
	require.Equal(t, -1, Run(cmd))
}

func makeFxError(_ *testing.T) error {
	app := fx.New(
		fx.Provide(func() (string, error) {
			return "", errors.New("uhoh")
		}),
		fx.Invoke(func(_ string) {}),
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
	displayError(makeFxError(t), &buf)
	require.Equal(t, "Error: uhoh\n", buf.String())
}
