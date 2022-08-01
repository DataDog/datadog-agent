// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	payload "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/process-agent/flags"
	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/process/events"
	"github.com/DataDog/datadog-agent/pkg/process/events/model"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	pullInterval     time.Duration
	eventsOutputJSON bool
)

const (
	defaultPullInterval     = time.Duration(5) * time.Second
	defaultEventsOutputJSON = false
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

// EventsPullCmd is a command to pull process lifecycle events
var EventsPullCmd = &cobra.Command{
	Use:          "pull",
	Short:        "Periodically pull process lifecycle events. This feature is currently in alpha version and needs root privilege to run.",
	RunE:         runEventStore,
	SilenceUsage: true,
}

func init() {
	EventsCmd.AddCommand(EventsListenCmd, EventsPullCmd)
	EventsListenCmd.Flags().BoolVar(&eventsOutputJSON, "json", defaultEventsOutputJSON, "Output events as JSON")
	EventsPullCmd.Flags().BoolVar(&eventsOutputJSON, "json", defaultEventsOutputJSON, "Output events as JSON")
	EventsPullCmd.Flags().DurationVarP(&pullInterval, "tick", "t", defaultPullInterval, "The period between 2 consecutive pulls to fetch process events")
}

func bootstrapEventsCmd(cmd *cobra.Command) error {
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

	return nil
}

func printEvents(events ...*model.ProcessEvent) error {
	// Return early to avoid printing new lines without any event
	if len(events) == 0 {
		return nil
	}

	if eventsOutputJSON {
		return printEventsJSON(events)
	}

	fmtEvents := fmtProcessEvents(events)
	procCollector := &payload.CollectorProcEvent{Events: fmtEvents}
	msgs := []payload.MessageBody{procCollector}
	return checks.HumanFormatProcessEvents(msgs, os.Stdout, false)
}

func printEventsJSON(events []*model.ProcessEvent) error {
	for _, e := range events {
		b, err := json.MarshalIndent(e, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal error: %s", err)
		}
		fmt.Println(string(b))
	}

	return nil
}

func runEventListener(cmd *cobra.Command, args []string) error {
	err := bootstrapEventsCmd(cmd)
	if err != nil {
		return err
	}

	// Create a handler to print the collected event to stdout
	handler := func(e *model.ProcessEvent) {
		err = printEvents(e)
		if err != nil {
			log.Error(err)
		}
	}

	l, err := events.NewListener(handler)
	if err != nil {
		return err
	}

	exit := make(chan struct{})
	go util.HandleSignals(exit)
	l.Run()

	<-exit
	l.Stop()
	log.Flush()

	return nil
}

func runEventStore(cmd *cobra.Command, args []string) error {
	err := bootstrapEventsCmd(cmd)
	if err != nil {
		return err
	}

	store, err := events.NewRingStore(&statsd.NoOpClient{})
	if err != nil {
		return err
	}

	l, err := events.NewListener(func(e *model.ProcessEvent) {
		// push events to the store asynchronously without checking for errors
		_ = store.Push(e, nil)
	})
	if err != nil {
		return err
	}

	store.Run()
	l.Run()

	exit := make(chan struct{})
	go util.HandleSignals(exit)

	ticker := time.NewTicker(pullInterval)
	defer ticker.Stop()
	go func() {
		for {
			select {
			case <-ticker.C:
				events, err := store.Pull(context.Background(), time.Second)
				if err != nil {
					log.Error(err)
					continue
				}

				err = printEvents(events...)
				if err != nil {
					log.Error(err)
				}
			case <-exit:
				return
			}
		}
	}()

	<-exit
	l.Stop()
	store.Stop()
	log.Flush()

	return nil
}

// fmtProcessEvents formats process lifecyle events as an agent payload
// The function in the checks package can't be exported without breaking the android build for the core-agent
// TODO: fix it
func fmtProcessEvents(events []*model.ProcessEvent) []*payload.ProcessEvent {
	payloadEvents := make([]*payload.ProcessEvent, 0, len(events))

	for _, e := range events {
		pE := &payload.ProcessEvent{
			CollectionTime: e.CollectionTime.UnixNano(),
			Pid:            e.Pid,
			ContainerId:    e.ContainerID,
			Command: &payload.Command{
				Exe:  e.Exe,
				Args: e.Cmdline,
				Ppid: int32(e.Ppid),
			},
			User: &payload.ProcessUser{
				Name: e.Username,
				Uid:  int32(e.UID),
				Gid:  int32(e.GID),
			},
		}

		switch e.EventType {
		case model.Exec:
			pE.Type = payload.ProcEventType_exec
			exec := &payload.ProcessExec{
				ForkTime: e.ForkTime.UnixNano(),
				ExecTime: e.ExecTime.UnixNano(),
			}
			pE.TypedEvent = &payload.ProcessEvent_Exec{Exec: exec}
		case model.Exit:
			pE.Type = payload.ProcEventType_exit
			exit := &payload.ProcessExit{
				ExecTime: e.ExecTime.UnixNano(),
				ExitTime: e.ExitTime.UnixNano(),
				ExitCode: int32(e.ExitCode),
			}
			pE.TypedEvent = &payload.ProcessEvent_Exit{Exit: exit}
		default:
			log.Error("Unexpected event type, dropping it")
			continue
		}

		payloadEvents = append(payloadEvents, pE)
	}

	return payloadEvents
}
