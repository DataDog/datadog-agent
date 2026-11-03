// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !darwin && !windows

package daemon

import (
	"github.com/DataDog/datadog-agent/cmd/installer/command"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

// daemonEnabledOnPlatform reports whether this platform lets the daemon run with the given
// configuration. Only macOS requires an opt-in.
func daemonEnabledOnPlatform(model.Reader) bool {
	return true
}

// daemonEnabledBeforeStart lets the daemon start outside macOS, where startDaemon checks
// remote_updates once the daemon's components are built, as it always has.
func daemonEnabledBeforeStart(*command.GlobalParams) (bool, error) {
	return true, nil
}
