// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync/atomic"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"

	"github.com/DataDog/datadog-agent/cmd/process-agent/flags"
	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/process/events/model"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/security/api"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// EventsCmd is a command to interact with process lifecycle events
var EventsCmd = &cobra.Command{
	Use:          "events",
	Short:        "Interact with process lifecycle events. This feature is currently in alpha version and needs root privilege to run.",
	SilenceUsage: true,
}

// EventsListenCmd is a command to listen for process lifecycle events
var EventsListenCmd = &cobra.Command{
	Use:          "listen",
	Short:        "Open a session to listen for process lifecycle events. This feature is currently in alpha version and needs root privilege to run.",
	RunE:         runEventListener,
	SilenceUsage: true,
}

func init() {
	EventsCmd.AddCommand(EventsListenCmd)
}

func runEventListener(cmd *cobra.Command, args []string) error {
	ddconfig.InitSystemProbeConfig(ddconfig.Datadog)

	configPath := cmd.Flag(flags.CfgPath).Value.String()
	var sysprobePath string
	if cmd.Flag(flags.SysProbeConfig) != nil {
		sysprobePath = cmd.Flag(flags.SysProbeConfig).Value.String()
	}

	if err := config.LoadConfigIfExists(configPath); err != nil {
		return log.Criticalf("Error parsing config: %s", err)
	}

	// Load system-probe.yaml file and merge it to the global Datadog config
	sysCfg, err := sysconfig.Merge(sysprobePath)
	if err != nil {
		return log.Critical(err)
	}

	// Set up logger
	_, err = config.NewAgentConfig(loggerName, configPath, sysCfg)
	if err != nil {
		return log.Criticalf("Error parsing config: %s", err)
	}

	// Create a gRPC client and connect to system-probe to listen for process events
	socketPath := ddconfig.Datadog.GetString("runtime_security_config.socket")
	if socketPath == "" {
		return errors.New("runtime_security_config.socket must be set")
	}

	conn, err := grpc.Dial(socketPath, grpc.WithInsecure(), grpc.WithContextDialer(func(ctx context.Context, url string) (net.Conn, error) {
		return net.Dial("unix", url)
	}))
	if err != nil {
		return err
	}

	client := api.NewSecurityModuleClient(conn)
	var connected, running atomic.Value
	connected.Store(false)

	exit := make(chan struct{})
	go util.HandleSignals(exit)
	go func() {
		<-exit
		log.Info("Stopping listening for process events")
		running.Store(false)
	}()

	log.Info("Start listening for process events")
	logTicker := newLogBackoffTicker()
	running.Store(true)
	for running.Load() == true {
		stream, err := client.GetProcessEvents(context.Background(), &api.GetProcessEventParams{})
		if err != nil {
			connected.Store(false)

			select {
			case <-logTicker.C:
				log.Warnf("Error while connecting to the runtime-security module: %v", err)
			default:
				// do nothing
			}

			// retry in 2 seconds
			time.Sleep(2 * time.Second)
			continue
		}

		if connected.Load() != true {
			connected.Store(true)

			log.Info("Successfully connected to the runtime-security module")
		}

		for {
			in, err := stream.Recv()
			if err == io.EOF || in == nil {
				break
			}

			var e model.ProcessMonitoringEvent
			if _, err := e.UnmarshalMsg(in.Data); err != nil {
				log.Error("Could not unmarshal process event: ", err.Error())
			}

			b, err := json.MarshalIndent(model.ProcessMonitoringtoProcessEvent(&e), "", "  ")
			if err != nil {
				log.Error("Error while marshalling process event: ", err.Error())
			}
			fmt.Println(string(b))
		}
	}

	logTicker.Stop()
	return nil
}

// newLogBackoffTicker returns a ticker based on an exponential backoff, used to trigger connection error logs
func newLogBackoffTicker() *backoff.Ticker {
	expBackoff := backoff.NewExponentialBackOff()
	expBackoff.InitialInterval = 2 * time.Second
	expBackoff.MaxInterval = 60 * time.Second
	expBackoff.MaxElapsedTime = 0
	expBackoff.Reset()
	return backoff.NewTicker(expBackoff)
}
