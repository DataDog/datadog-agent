// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package usermod

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
)

func TestUsermodFlagsAndCapability(t *testing.T) {
	for _, test := range []struct {
		args  []string
		valid bool
	}{
		{[]string{"-aG", "docker", "dd-agent"}, true},
		{[]string{"--append", "--groups=docker", "dd-agent"}, true},
		{[]string{"-G", "docker", "-a", "dd-agent"}, true},
		{[]string{"-G", "docker", "dd-agent"}, false},
		{[]string{"--append=false", "-G", "docker", "dd-agent"}, false},
		{[]string{"-aG", "docker,root", "dd-agent"}, false},
		{[]string{"-aG", "docker", "root"}, false},
		{[]string{"-aG", "docker", "dd-agent", "extra"}, false},
		{[]string{"-aG", "", "dd-agent"}, false},
		{[]string{"-a"}, false},
	} {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			fs := pflag.NewFlagSet("usermod", pflag.ContinueOnError)
			run := makeFlags(fs)
			require.NoError(t, fs.Parse(test.args))
			var out, stderr bytes.Buffer
			called := false
			call := &builtins.CallContext{Stdout: &out, Stderr: &stderr, RemediationMode: true, AddDDAgentToDocker: func(context.Context) error { called = true; return nil }}
			result := run(context.Background(), call, fs.Args())
			require.Equal(t, test.valid, called)
			require.Equal(t, test.valid, result.Code == 0)
			call.RemediationMode = false
			called = false
			require.NotZero(t, run(context.Background(), call, fs.Args()).Code)
			require.False(t, called)
		})
	}
	for _, flag := range []string{"-d", "-m", "-s", "-u", "-g", "-p", "-L", "-U", "-e", "-f", "-c", "-l", "-o", "-R", "-P", "-Z", "--add-subuids", "--add-subgids"} {
		fs := pflag.NewFlagSet("usermod", pflag.ContinueOnError)
		makeFlags(fs)
		require.Error(t, fs.Parse([]string{flag, "value"}))
	}
}
func TestUsermodHelpAndErrors(t *testing.T) {
	var out, stderr bytes.Buffer
	call := &builtins.CallContext{Stdout: &out, Stderr: &stderr, RemediationMode: true}
	fs := pflag.NewFlagSet("usermod", pflag.ContinueOnError)
	run := makeFlags(fs)
	require.NoError(t, fs.Parse([]string{"--help"}))
	require.Zero(t, run(context.Background(), call, fs.Args()).Code)
	require.Contains(t, out.String(), "NOT crash-atomic")
	require.Contains(t, out.String(), "-G, --groups")
	fs = pflag.NewFlagSet("usermod", pflag.ContinueOnError)
	run = makeFlags(fs)
	require.NoError(t, fs.Parse([]string{"-aG", "docker", "dd-agent"}))
	require.NotZero(t, run(context.Background(), call, fs.Args()).Code)
	require.Contains(t, stderr.String(), "authorized elevation")
	call.AddDDAgentToDocker = func(context.Context) error { return errors.New("write failed; recovery copy retained") }
	require.NotZero(t, run(context.Background(), call, fs.Args()).Code)
	require.Contains(t, stderr.String(), "write failed")
}
