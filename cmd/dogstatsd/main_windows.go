// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"context"
	_ "net/http/pprof"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/cmd/dogstatsd/command"
	"github.com/DataDog/datadog-agent/cmd/dogstatsd/subcommands/start"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	dogstatsdServer "github.com/DataDog/datadog-agent/comp/dogstatsd/server"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/messagestrings"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/servicemain"
)

var (
	defaultLogFile = "c:\\programdata\\datadog\\logs\\dogstatsd.log"

	// DefaultConfPath points to the folder containing datadog.yaml
	DefaultConfPath = "c:\\programdata\\datadog"
)

func init() {
	pd, err := winutil.GetProgramDataDirForProduct("Datadog Dogstatsd")
	if err == nil {
		DefaultConfPath = pd
		defaultLogFile = filepath.Join(pd, "logs", "dogstatsd.log")
	} else {
		winutil.LogEventViewer(ServiceName, messagestrings.MSG_WARNING_PROGRAMDATA_ERROR, defaultLogFile)
	}
}

// ServiceName is the name of the service in service control manager
const ServiceName = "dogstatsd"

func main() {
	// set the Agent flavor
	flavor.SetFlavor(flavor.Dogstatsd)

	if servicemain.RunningAsWindowsService() {
		servicemain.Run(&service{})
		return
	}
	defer pkglog.Flush()

	if err := command.MakeRootCommand(defaultLogFile).Execute(); err != nil {
		pkglog.Error(err)
		os.Exit(-1)
	}
}

type service struct{}

func (s *service) Name() string {
	return ServiceName
}

func (s *service) Init() error {
	// Nothing to do, kept empty for compatibility with previous implementation.
	return nil
}

func (s *service) Run(ctx context.Context) error {
	pkglog.Infof("Service control function")

	ctx, cancel := context.WithCancel(ctx)
	cliParams := &start.CLIParams{}

	return start.RunDogstatsdFct(
		cliParams,
		DefaultConfPath,
		defaultLogFile,
		func(config config.Component, log log.Component, params *start.Params, server dogstatsdServer.Component, sharedForwarder defaultforwarder.Component, demux *aggregator.AgentDemultiplexer) error {
			components := &start.DogstatsdComponents{
				DogstatsdServer: server,
			}
			defer start.StopAgent(cancel, components)

			err := start.RunDogstatsd(ctx, cliParams, config, log, params, components, demux)
			if err != nil {
				log.Errorf("Failed to start agent %v", err)
				return err
			}

			// Wait for stop signal
			<-ctx.Done()

			log.Infof("Initiating service shutdown")
			return nil
		})
}
