// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/cmd/internal/runcmd"
	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/cmd/system-probe/subcommands"
	runsubcmd "github.com/DataDog/datadog-agent/cmd/system-probe/subcommands/run"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/servicemain"
)

type service struct {
	servicemain.DefaultSettings
	errChan <-chan error
	ctxChan chan context.Context
}

func (s *service) Name() string {
	return config.ServiceName
}

func (s *service) Init() error {
	s.ctxChan = make(chan context.Context)

	errChan, err := runsubcmd.StartSystemProbeWithDefaults(s.ctxChan)
	if err != nil {
		if errors.Is(err, runsubcmd.ErrNotEnabled) {
			return fmt.Errorf("%w: %w", servicemain.ErrCleanStopAfterInit, err)
		}
		return err
	}

	s.errChan = errChan

	return nil
}

func (s *service) Run(ctx context.Context) error {
	// send context to background agent goroutine so we can stop the agent
	s.ctxChan <- ctx
	// wait for agent to stop
	return <-s.errChan
}

func main() {
	// if command line arguments are supplied, even in a non-interactive session,
	// then just execute that.  Used when the service is executing the executable,
	// for instance to trigger a restart.
	if len(os.Args) == 1 {
		if servicemain.RunningAsWindowsService() {
			servicemain.Run(&service{})
			return
		}
	}
	defer log.Flush()

	rootCmd := command.MakeCommand(subcommands.SysprobeSubcommands())
	command.SetDefaultCommandIfNonePresent(rootCmd)
	os.Exit(runcmd.Run(rootCmd))
}
