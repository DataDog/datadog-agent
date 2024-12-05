// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package packages

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"

	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

// removeDebRPMPackage removes a package installed via deb/rpm package manager
// It doesn't remove dependencies or purge as we want to keep existing configuration files
// and reinstall the package using the installer.
func removeDebRPMPackage(ctx context.Context, pkg string) (err error) {
	span, _ := tracer.StartSpanFromContext(ctx, "remove_deb_rpm_package")
	defer func() { span.Finish(tracer.WithError(err)) }()
	// Compute the right command depending on the package manager
	var cmd *exec.Cmd
	if _, pathErr := exec.LookPath("dpkg"); pathErr == nil {
		cmd = exec.Command("dpkg", "-r", agentPackage)
	} else if _, pathErr := exec.LookPath("rpm"); pathErr == nil {
		// Check if package exist, else the command will fail
		pkgErr := exec.Command("rpm", "-q", agentPackage).Run()
		if pkgErr == nil {
			cmd = exec.Command("rpm", "-e", "--nodeps", agentPackage)
		}
	}

	if cmd == nil {
		// If we can't find a package manager we'll simply remove the directory
		// where the package is expected to be installed
		switch pkg {
		case agentPackage:
			cmd = exec.Command("rm", "-rf", pathOldAgent)
		default:
			return nil
		}
	}

	// Run the command
	var buf bytes.Buffer
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to uninstall deb/rpm package %s (%w): %s", pkg, err, buf.String())
	}
	return nil
}
