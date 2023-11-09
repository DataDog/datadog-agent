// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package inject

import (
	"encoding/binary"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"syscall"
	"time"
	"unsafe"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/cws-injector/flags"
	"github.com/DataDog/datadog-agent/pkg/security/probe/erpc"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model/user_session"
)

const (
	// UserSessionDataMaxSize is the maximum size for the user session context
	UserSessionDataMaxSize = 1024
)

type injectCliParams struct {
	Data        string
	SessionType string
}

// Command returns the commands for the setup subcommand
func Command() []*cobra.Command {
	var params injectCliParams

	injectCmd := &cobra.Command{
		Use:   "inject",
		Short: "Forwards the input user context to the CWS agent with eRPC",
		RunE: func(cmd *cobra.Command, args []string) error {
			return injectUserSessionCmd(args, &params)
		},
	}

	injectCmd.Flags().StringVar(&params.Data, flags.Data, "", "The user session data to inject")
	injectCmd.Flags().StringVar(&params.SessionType, flags.SessionType, "", "The user session type to inject. Possible values are: [k8s]")
	_ = injectCmd.MarkFlagRequired(flags.SessionType)

	return []*cobra.Command{injectCmd}
}

// injectUserSessionCmd handles inject commands
func injectUserSessionCmd(args []string, params *injectCliParams) error {
	if err := injectUserSession(params); err != nil {
		// log error but do not return now, we need to execute the user provided command
		fmt.Println(err.Error())
	}

	// check user input
	if len(args) == 0 {
		fmt.Printf("cws-injector: empty command, nothing to run ... if the problem persists, disable `admission_controller.cws_instrumentation.enabled` in the Datadog Agent config and try again.\n")
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

// injectUserSession copies the cws-injector binary to the provided target directory
func injectUserSession(params *injectCliParams) error {
	// sanitize user input
	if len(params.Data) > UserSessionDataMaxSize {
		return fmt.Errorf("user session context too long: %d", len(params.Data))
	}
	user_session.InitUserSessionTypes()
	sessionType := user_session.UserSessionTypes[params.SessionType]
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
	byteOrder := getHostByteOrder()

	// send the user session to the CWS agent
	cursor := 0
	var segmentSize int
	segmentCursor := uint8(1)
	for cursor < len(params.Data) || (len(params.Data) == cursor && cursor == 0) {
		req := erpc.NewERPCRequest(263)
		req.OP = erpc.UserSessionContextOp

		byteOrder.PutUint64(req.Data[0:8], id)
		req.Data[8] = segmentCursor
		req.Data[9] = uint8(sessionType)

		if 246 < len(params.Data)-cursor {
			segmentSize = 246
		} else {
			segmentSize = len(params.Data) - cursor
		}
		copy(req.Data[16:], params.Data[cursor:cursor+segmentSize])

		// issue eRPC calls
		if err = client.Request(req); err != nil {
			return fmt.Errorf("eRPC request failed: %v", err)
		}

		cursor += segmentSize
		segmentCursor++

		if cursor == 0 {
			// handle cases where we don't have any user session data, but still use this mechanism to track a session
			cursor++
		}
	}
	return nil
}

func getHostByteOrder() binary.ByteOrder {
	var i int32 = 0x01020304
	u := unsafe.Pointer(&i)
	pb := (*byte)(u)
	b := *pb
	if b == 0x04 {
		return binary.LittleEndian
	}

	return binary.BigEndian
}
