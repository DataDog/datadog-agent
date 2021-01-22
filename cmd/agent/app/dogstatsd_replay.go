// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018-2020 Datadog, Inc.

package app

import (
	"fmt"
	"net"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd/debug"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	dsdReplayFilePath string
	dsdTaggerFilePath string
)

func init() {
	AgentCmd.AddCommand(dogstatsdReplayCmd)
	dogstatsdReplayCmd.Flags().StringVarP(&dsdReplayFilePath, "file", "f", "", "Input file with TCP traffic to replay.")
	dogstatsdReplayCmd.Flags().StringVarP(&dsdTaggerFilePath, "tagger", "t", "", "Input file with TCP traffic to replay.")
}

var dogstatsdReplayCmd = &cobra.Command{
	Use:   "dogstatsd-replay",
	Short: "Replay dogstatsd traffic",
	Long:  ``,
	RunE: func(cmd *cobra.Command, args []string) error {

		if flagNoColor {
			color.NoColor = true
		}

		err := common.SetupConfigWithoutSecrets(confFilePath, "")
		if err != nil {
			return fmt.Errorf("unable to set up global agent configuration: %v", err)
		}

		err = config.SetupLogger(loggerName, config.GetEnvDefault("DD_LOG_LEVEL", "off"), "", "", false, true, false)
		if err != nil {
			fmt.Printf("Cannot setup logger, exiting: %v\n", err)
			return err
		}

		return dogstatsdReplay()
	},
}

func dogstatsdReplay() error {
	fmt.Printf("Replaying dogstatsd traffic...\n\n")
	s := config.Datadog.GetString("dogstatsd_socket")
	if s == "" {
		return fmt.Errorf("Dogstatsd UNIX socket disabled")
	}

	// TODO: tagger state probably belogs in the replay file anyways.
	// depth should be configurable....
	// reader, e := debug.NewTrafficCaptureReader(dsdReplayFilePath, dsdTaggerFilePath)
	depth := 10
	reader, e := debug.NewTrafficCaptureReader(dsdReplayFilePath, depth)
	if e != nil {
		return e
	}

	addr, err := net.ResolveUnixAddr("unix", s)
	if err != nil {
		return err
	}

	conn, err := net.DialUnix("unix", nil, addr)
	if err != nil {
		return err
	}

	// enable reading at natural rate
	go reader.Read()

	// TODO: cleanup shutdown
	for {
		select {
		case msg := <-reader.Traffic:
			_, _, err := conn.WriteMsgUnix(msg.Payload, msg.Ancillary, addr)
			if err != nil {
				return err
			}
		case <-reader.Shutdown:
			break
		}
	}

	return nil
}
