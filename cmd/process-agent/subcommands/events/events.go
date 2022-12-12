// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package events

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	payload "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/process-agent/command"
	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/process/events"
	"github.com/DataDog/datadog-agent/pkg/process/events/model"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	defaultPullInterval     = time.Duration(5) * time.Second
	defaultEventsOutputJSON = false
)

type cliParams struct {
	*command.GlobalParams

	pullInterval     time.Duration
	eventsOutputJSON bool
}

// Commands returns a slice of subcommands for the `events` command in the Process Agent
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &cliParams{
		GlobalParams: globalParams,
	}

	// eventsCmd is a command to interact with process lifecycle events
	eventsCmd := &cobra.Command{
		Use:          "events",
		Short:        "Interact with process lifecycle events. This feature is currently in alpha version and needs root privilege to run.",
		SilenceUsage: true,
	}

	// eventsListenCmd is a command to listen for process lifecycle events
	eventsListenCmd := &cobra.Command{
		Use:   "listen",
		Short: "Open a session to listen for process lifecycle events. This feature is currently in alpha version and needs root privilege to run.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(runEventListener,
				fx.Supply(cliParams),
			)
		},
		SilenceUsage: true,
	}

	// eventsPullCmd is a command to pull process lifecycle events
	eventsPullCmd := &cobra.Command{
		Use:   "pull",
		Short: "Periodically pull process lifecycle events. This feature is currently in alpha version and needs root privilege to run.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(runEventStore,
				fx.Supply(cliParams),
			)
		},
		SilenceUsage: true,
	}

	eventsCmd.AddCommand(eventsListenCmd, eventsPullCmd)
	eventsListenCmd.Flags().BoolVar(&cliParams.eventsOutputJSON, "json", defaultEventsOutputJSON, "Output events as JSON")
	eventsPullCmd.Flags().BoolVar(&cliParams.eventsOutputJSON, "json", defaultEventsOutputJSON, "Output events as JSON")
	eventsPullCmd.Flags().DurationVarP(&cliParams.pullInterval, "tick", "t", defaultPullInterval, "The period between 2 consecutive pulls to fetch process events")

	return []*cobra.Command{eventsCmd}
}

func bootstrapEventsCmd(cliParams *cliParams) error {
	ddconfig.InitSystemProbeConfig(ddconfig.Datadog)

	if err := config.LoadConfigIfExists(cliParams.ConfFilePath); err != nil {
		return log.Criticalf("Error parsing config: %s", err)
	}

	// Load system-probe.yaml file and merge it to the global Datadog config
	sysCfg, err := sysconfig.Merge(cliParams.SysProbeConfFilePath)
	if err != nil {
		return log.Critical(err)
	}

	// Set up logger
	_, err = config.NewAgentConfig(command.LoggerName, cliParams.ConfFilePath, sysCfg)
	if err != nil {
		return log.Criticalf("Error parsing config: %s", err)
	}

	return nil
}

func printEvents(cliParams *cliParams, events ...*model.ProcessEvent) error {
	// Return early to avoid printing new lines without any event
	if len(events) == 0 {
		return nil
	}

	if cliParams.eventsOutputJSON {
		return printEventsJSON(events)
	}

	fmtEvents := checks.FmtProcessEvents(events)
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

func runEventListener(cliParams *cliParams) error {
	err := bootstrapEventsCmd(cliParams)
	if err != nil {
		return err
	}

	// Create a handler to print the collected event to stdout
	handler := func(e *model.ProcessEvent) {
		err = printEvents(cliParams, e)
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

func runEventStore(cliParams *cliParams) error {
	err := bootstrapEventsCmd(cliParams)
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

	ticker := time.NewTicker(cliParams.pullInterval)
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

				err = printEvents(cliParams, events...)
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
