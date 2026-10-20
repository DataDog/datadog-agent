// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package executor

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

const (
	// fileCreatePipeInstance is FILE_CREATE_PIPE_INSTANCE (same value as FILE_APPEND_DATA).
	fileCreatePipeInstance = windows.FILE_APPEND_DATA

	// pipeClientAccess matches what par-control requests (PIPE_CLIENT_DESIRED_ACCESS
	// in dd-procmgr-client). Unlike FILE_GENERIC_WRITE, it cannot create instances.
	pipeClientAccess = windows.FILE_GENERIC_READ | windows.FILE_WRITE_DATA

	// pipeServerAccess is what go-winio needs to create each further instance.
	pipeServerAccess = windows.FILE_GENERIC_READ | windows.FILE_GENERIC_WRITE

	// pipeSecurityDescriptorTemplate is a protected DACL: full access for Local
	// System (SY) and Administrators (BA), server access for the pipe's owner
	// (OW, which also withholds its implicit WRITE_DAC), and client access for
	// the Agent account.
	pipeSecurityDescriptorTemplate = "D:P(A;;FA;;;SY)(A;;FA;;;BA)(A;;%#x;;;OW)(A;;%#x;;;%s)"
)

// broadSIDs are groups that must never stand in for the Agent account.
var broadSIDs = map[string]string{
	"S-1-1-0":      "Everyone",
	"S-1-5-7":      "Anonymous Logon",
	"S-1-5-11":     "Authenticated Users",
	"S-1-5-32-545": "Users",
	"S-1-5-32-546": "Guests",
}

// agentAccountSID resolves the datadogagent service account, which dd-procmgrd
// also runs par-control and the executor as. Replaced in tests.
var agentAccountSID = winutil.GetDDAgentUserSID

// Listen creates the executor's named pipe with pipeSecurityDescriptorTemplate.
// It fails rather than fall back to a broader or default DACL.
func Listen(address string) (net.Listener, error) {
	sd, err := pipeSecurityDescriptor()
	if err != nil {
		return nil, fmt.Errorf("could not build executor pipe security descriptor: %w", err)
	}
	return winio.ListenPipe(address, &winio.PipeConfig{SecurityDescriptor: sd})
}

func pipeSecurityDescriptor() (string, error) {
	sid, err := agentAccountSID()
	if err != nil {
		return "", fmt.Errorf("could not resolve the Agent service account: %w", err)
	}
	if sid == nil {
		return "", errors.New("could not resolve the Agent service account")
	}
	return formatPipeSecurityDescriptor(sid.String())
}

func formatPipeSecurityDescriptor(sid string) (string, error) {
	if name, ok := broadSIDs[sid]; ok {
		return "", fmt.Errorf("the Agent account resolved to %s (%s), which is not supported", name, sid)
	}
	return fmt.Sprintf(pipeSecurityDescriptorTemplate, pipeServerAccess, pipeClientAccess, sid), nil
}

// Dial connects to the executor's named pipe the way par-control does: with
// pipeClientAccess, at Identification impersonation level. A zero timeout
// means no timeout, as on Unix.
func Dial(ctx context.Context, address string, timeout time.Duration) (net.Conn, error) {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	return winio.DialPipeAccessImpLevel(ctx, address, pipeClientAccess, winio.PipeImpLevelIdentification)
}
