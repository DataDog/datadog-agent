// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package injectcmd holds the inject command of CWS injector
package injectcmd

import (
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/cws-instrumentation/flags"

	"github.com/DataDog/datadog-agent/pkg/security/probe/erpc"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model/usersession"
	"github.com/DataDog/datadog-agent/pkg/util/native"
)

const (
	// UserSessionDataMaxSize is the maximum size for the user session context
	UserSessionDataMaxSize = 1024
)

// InjectCliParams contains the parameters of the inject command
type InjectCliParams struct {
	Data        string
	SessionType string
}

// Command returns the commands for the setup subcommand
func Command() []*cobra.Command {
	var params InjectCliParams

	injectCmd := &cobra.Command{
		Use:   "inject",
		Short: "Forwards the input user context to the CWS agent with eRPC",
		RunE: func(cmd *cobra.Command, args []string) error {
			return InjectUserSessionCmd(args, &params)
		},
	}

	injectCmd.Flags().StringVar(&params.Data, flags.Data, "", "The user session data to inject")
	injectCmd.Flags().StringVar(&params.SessionType, flags.SessionType, "", "The user session type to inject. Possible values are: [k8s]")
	_ = injectCmd.MarkFlagRequired(flags.SessionType)

	return []*cobra.Command{injectCmd}
}

// InjectUserSessionCmd handles inject commands
func InjectUserSessionCmd(args []string, params *InjectCliParams) error {
	if err := injectUserSession(params); err != nil {
		// log error but do not return now, we need to execute the user provided command
		fmt.Println(err.Error())
	}

	// check user input
	if len(args) == 0 {
		fmt.Printf("cws-instrumentation: empty command, nothing to run ... if the problem persists, disable `admission_controller.cws_instrumentation.enabled` in the Datadog Agent config and try again.\n")
		return nil
	}

	// look for the input binary in PATH
	// This is done to copy the logic in: https://github.com/opencontainers/runc/blob/acab6f6416142a302f9c324b3f1b66a1e46e29ef/libcontainer/standard_init_linux.go#L201
	resolvedPath, err := exec.LookPath(args[0])
	if err != nil {
		// args[0] not found in PATH, default to the raw input
		resolvedPath = args[0]
	}

	return syscall.Exec(resolvedPath, args, os.Environ())
}

// injectUserSession copies the cws-instrumentation binary to the provided target directory
func injectUserSession(params *InjectCliParams) error {
	// sanitize user input
	if len(params.Data) > UserSessionDataMaxSize {
		return fmt.Errorf("user session context too long: %d", len(params.Data))
	}
	usersession.InitUserSessionTypes()
	sessionType := usersession.UserSessionTypes[params.SessionType]
	if sessionType == 0 {
		return fmt.Errorf("unknown user session type: %v", params.SessionType)
	}

	// create a new eRPC client
	client, err := erpc.NewERPC()
	if err != nil {
		return fmt.Errorf("couldn't create eRPC client: %w", err)
	}

	// generate random ID for this session
	id := (uint64(rand.Uint32()) << 32) + uint64(time.Now().Unix())

	// send the user session to the CWS agent
	cursor := 0
	var segmentSize int
	segmentCursor := uint8(1)
	for cursor < len(params.Data) || (len(params.Data) == 0 && cursor == 0) {
		req := erpc.NewERPCRequest(erpc.UserSessionContextOp)

		native.Endian.PutUint64(req.Data[0:8], id)
		req.Data[8] = segmentCursor
		// padding
		req.Data[16] = uint8(sessionType)

		if erpc.ERPCDefaultDataSize-17 < len(params.Data)-cursor {
			segmentSize = erpc.ERPCDefaultDataSize - 17
		} else {
			segmentSize = len(params.Data) - cursor
		}
		copy(req.Data[17:], params.Data[cursor:cursor+segmentSize])

		// issue eRPC calls
		if err = client.Request(req); err != nil {
			return fmt.Errorf("eRPC request failed: %v", err)
		}

		cursor += segmentSize
		segmentCursor++

		if cursor == 0 {
			// handle cases where we don't have any user session data, but still use this mechanism to start a session
			cursor++
		}
	}
	return nil
}
