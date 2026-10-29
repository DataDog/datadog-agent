// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package compliance

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/config/env"
)

// XCCDFProbeRoot returns the root of the host filesystem for oscap-io to
// probe: $HOST_ROOT, or /host, when the agent runs in a container, and the
// empty string, for the running system, otherwise.
func XCCDFProbeRoot() string {
	if !env.IsContainerized() {
		return ""
	}
	if root := os.Getenv("HOST_ROOT"); root != "" {
		return root
	}
	return "/host"
}

// oscapIOCommand returns the command that runs oscap-io on the datastream
// file, probing root when it is set. The command is the compliance
// oscap-exec subcommand of the running binary, which drops capabilities
// before it executes oscap-io.
func oscapIOCommand(ctx context.Context, file, root string) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, "/proc/self/exe", "compliance", "oscap-exec", file)
	cmd.Dir = filepath.Dir(file)
	cmd.Env = os.Environ()
	if root != "" {
		cmd.Env = append(cmd.Env, "OSCAP_PROBE_ROOT="+root)
	}
	return cmd, nil
}
