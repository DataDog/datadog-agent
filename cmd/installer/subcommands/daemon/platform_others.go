// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !darwin && !windows

package daemon

import (
	"github.com/DataDog/datadog-agent/cmd/installer/command"
)

// daemonEnabledBeforeStart lets the daemon start outside macOS, where the updater component
// checks remote_updates and remote configuration while the daemon's components are built.
func daemonEnabledBeforeStart(*command.GlobalParams) (bool, error) {
	return true, nil
}
