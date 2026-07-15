// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

//nolint:revive // TODO(SERV) Fix revive linter
package mode

import (
	"errors"
	"os"

	serverlessLog "github.com/DataDog/datadog-agent/cmd/serverless-init/log"
)

// Conf contains the configuration for the mode in which the serverless-init agent should run.
// serverless-init itself is not supported on windows (see cmd/serverless-init/main_windows.go);
// this type exists only so that packages like cloudservice, which reference mode.Conf, still
// build on windows.
type Conf struct {
	LoggerName                    string
	Runner                        func(logConfig *serverlessLog.Config) error
	TagVersionMode                string
	TagVersionModeEnhancedMetrics string
	SidecarMode                   bool
	EnvDefaults                   map[string]string
}

// ProcessHooks mirrors the init-container ProcessHooks type so that packages
// which construct it (e.g. cloudservice.MicroVM) still build on windows.
// serverless-init is not supported on windows, so these hooks are never invoked.
type ProcessHooks struct {
	OnProcess func(*os.Process)
	OnAlive   func()
	OnDead    func()
}

// RunInit is unsupported on windows.
func RunInit(_ *serverlessLog.Config, _ *ProcessHooks) error {
	return errors.New("serverless-init is not supported on windows")
}

// RunSidecar is unsupported on windows.
func RunSidecar(_ *serverlessLog.Config) error {
	return errors.New("serverless-init is not supported on windows")
}
