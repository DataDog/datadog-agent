// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(PROC) Fix revive linter
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
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/process"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	"github.com/DataDog/datadog-agent/pkg/process/events"
	"github.com/DataDog/datadog-agent/pkg/process/events/model"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
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

type dependencies struct {
	fx.In

	Params *cliParams

	Config         config.Component
	SysProbeConfig sysprobeconfig.Component
	Log            log.Component
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
				fx.Supply(cliParams, command.GetCoreBundleParamsForOneShot(globalParams)),
				core.Bundle(),
				process.Bundle(),
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
				fx.Supply(cliParams, command.GetCoreBundleParamsForOneShot(globalParams)),
				core.Bundle(),
				process.Bundle(),
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

func runEventListener(deps dependencies) error {
	// Create a handler to print the collected event to stdout
	handler := func(e *model.ProcessEvent) {
		err := printEvents(deps.Params, e)
		if err != nil {
			_ = deps.Log.Error(err)
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
	deps.Log.Flush()

	return nil
}

func runEventStore(deps dependencies) error {
	store, err := events.NewRingStore(deps.Config, &statsd.NoOpClient{})
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

	ticker := time.NewTicker(deps.Params.pullInterval)
	defer ticker.Stop()
	go func() {
		for {
			select {
			case <-ticker.C:
				events, err := store.Pull(context.Background(), time.Second)
				if err != nil {
					_ = deps.Log.Error(err)
					continue
				}

				err = printEvents(deps.Params, events...)
				if err != nil {
					_ = deps.Log.Error(err)
				}
			case <-exit:
				return
			}
		}
	}()

	<-exit
	l.Stop()
	store.Stop()
	deps.Log.Flush()

	return nil
}
