// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package truecmd implements the true builtin command.
//
// true — return a successful exit status
//
// Usage: true
//
// Do nothing and return an exit status of 0. All arguments are ignored.
//
// Exit codes:
//
//	0  Always succeeds.
package truecmd

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

// Cmd is the true builtin command descriptor.
var Cmd = builtins.Command{
	Name:        "true",
	Description: "return successful exit status",
	Help: `true: true
    Return a successful result.

    Exit with a status code indicating success.`,
	MakeFlags: builtins.NoFlags(run),
}

func run(_ context.Context, _ *builtins.CallContext, _ []string) builtins.Result {
	return builtins.Result{}
}
