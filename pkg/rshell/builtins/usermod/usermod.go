// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package usermod implements the fixed Docker-group remediation POC.
package usermod

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

// Cmd exposes only append-dd-agent-to-docker, not general account management.
var Cmd = builtins.Command{
	Name:            "usermod",
	Description:     "POC: append dd-agent to docker (Linux, elevated remediation only)",
	RemediationOnly: true,
	MakeFlags:       makeFlags,
}

func makeFlags(fs *builtins.FlagSet) builtins.HandlerFunc {
	appendMember := fs.BoolP("append", "a", false, "append membership; required")
	group := fs.StringP("groups", "G", "", "supplementary group; only docker is supported")
	help := fs.BoolP("help", "h", false, "print usage and exit")
	return func(ctx context.Context, call *builtins.CallContext, args []string) builtins.Result {
		if !call.RemediationMode {
			call.Errf("usermod: remediation mode required\n")
			return builtins.Result{Code: 1}
		}
		if *help {
			call.Out("Usage: usermod -aG docker dd-agent\n")
			call.Out("POC: append only dd-agent to the existing local docker group (Linux).\n")
			call.Out("Requires authorized elevation and writable /etc in AllowedPaths.\n")
			call.Out("In-place updates are NOT crash-atomic; run with account editors idle.\n")
			call.Out("Recovery copies are retained as /etc/*.rshell-usermod-* (0600).\n")
			fs.SetOutput(call.Stdout)
			fs.PrintDefaults()
			return builtins.Result{}
		}
		if !*appendMember || *group != "docker" || len(args) != 1 || args[0] != "dd-agent" {
			call.Errf("usermod: only -aG docker dd-agent is supported\n")
			return builtins.Result{Code: 1}
		}
		if call.AddDDAgentToDocker == nil {
			call.Errf("usermod: authorized elevation and an AllowedPaths sandbox are required\n")
			return builtins.Result{Code: 1}
		}
		if err := call.AddDDAgentToDocker(ctx); err != nil {
			call.Errf("usermod: %s\n", err)
			return builtins.Result{Code: 1}
		}
		return builtins.Result{}
	}
}
