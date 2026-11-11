// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package daemon

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/installer/command"
	pid "github.com/DataDog/datadog-agent/comp/core/pid/def"
	localapi "github.com/DataDog/datadog-agent/comp/updater/localapi/def"
	telemetry "github.com/DataDog/datadog-agent/comp/updater/telemetry/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func runFxWrapper(global *command.GlobalParams) error {
	enabled, err := daemonEnabledBeforeStart(global)
	if err != nil || !enabled {
		return err
	}
	err = fxutil.Run(
		getCommonFxOption(global),
		fx.Invoke(func(_ localapi.Component) {}),
		fx.Invoke(func(_ pid.Component) {}),
		fx.Invoke(func(_ telemetry.Component) {}),
		fx.Invoke(releaseMemory),
	)
	if isDaemonDisabled(err) {
		log.Infof("Datadog installer is not enabled, exiting")
		return nil
	}
	return err
}
