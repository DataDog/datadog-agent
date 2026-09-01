// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package macpkg

import (
	"context"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const systemInstallerPath = "/usr/sbin/installer"

// SystemInstaller runs the macOS system installer over a verified .pkg.
type SystemInstaller struct {
	// Runner executes the installer. Nil runs the real binary; tests substitute a recorder.
	Runner Runner
}

// Install extracts the package onto the boot volume.
//
// -target is a volume, not a directory: the payload's destination is baked into the package at
// build time and the installer cannot be told otherwise. That is why packaging is per-version and
// why no Datadog macOS package is relocatable -- a relocatable package would let the installer
// place code outside the pool, and the pool is the only thing the façades can resolve through.
func (i SystemInstaller) Install(ctx context.Context, pkgPath string) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "macpkg.install")
	defer func() { span.Finish(err) }()
	span.SetTag("package.path", pkgPath)

	out, err := i.run(ctx, systemInstallerPath, "-pkg", pkgPath, "-target", "/")
	if err != nil {
		// The installer's own output is the only description of what went wrong, and it does
		// not survive into the error, so it goes to the log before being wrapped.
		log.Errorf("the system installer failed for %s: %s", pkgPath, strings.TrimSpace(string(out)))
		return fmt.Errorf("could not install %s: %w (%s)", pkgPath, err, strings.TrimSpace(string(out)))
	}
	log.Infof("installed %s: %s", pkgPath, strings.TrimSpace(string(out)))
	return nil
}

func (i SystemInstaller) run(ctx context.Context, name string, args ...string) ([]byte, error) {
	if i.Runner != nil {
		return i.Runner(ctx, name, args...)
	}
	return telemetry.CommandContext(ctx, name, args...).CombinedOutput()
}
