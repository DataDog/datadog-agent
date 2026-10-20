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
	"strings"
	"time"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

const (
	// fileCreatePipeInstance is FILE_CREATE_PIPE_INSTANCE, which shares its
	// value with FILE_APPEND_DATA.
	fileCreatePipeInstance = windows.FILE_APPEND_DATA

	// pipeClientAccess is what par-control requests when it opens the pipe
	// (PIPE_CLIENT_DESIRED_ACCESS in dd-procmgr-client) and all the installed
	// Agent account is granted. FILE_GENERIC_WRITE is avoided because it
	// includes FILE_CREATE_PIPE_INSTANCE.
	pipeClientAccess = windows.FILE_GENERIC_READ | windows.FILE_WRITE_DATA

	// pipeServerAccess is what go-winio requests each time it creates another
	// pipe instance for Accept (GENERIC_READ | GENERIC_WRITE), which also
	// requires FILE_CREATE_PIPE_INSTANCE. It is granted only to the pipe's
	// owner, the identity that created the first instance.
	pipeServerAccess = windows.FILE_GENERIC_READ | windows.FILE_GENERIC_WRITE

	// pipeSecurityDescriptorTemplate is a protected DACL (P) that allows Local
	// System (SY) and Administrators (BA) full access, the pipe's owner (OW,
	// OWNER RIGHTS) server access, and the installed Agent account client
	// access. The OW entry also withholds the owner's implicit WRITE_DAC.
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

// agentAccountSID resolves the account the datadogagent service runs as, which
// is also the account dd-procmgrd starts par-control and the executor as. It
// reads the service configuration, so custom service accounts are honored.
// Tests replace it so they do not depend on an installed Agent.
var agentAccountSID = winutil.GetDDAgentUserSID

// Listen creates the executor's listening named pipe, restricted to Local
// System, Administrators, its creator and the installed Agent account. It
// fails rather than creating a pipe with a broader or default DACL.
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
	if !strings.HasPrefix(sid, "S-") {
		return "", fmt.Errorf("invalid Agent account SID %q", sid)
	}
	if name, ok := broadSIDs[sid]; ok {
		return "", fmt.Errorf("the Agent account resolved to %s (%s), which is not supported", name, sid)
	}
	return fmt.Sprintf(pipeSecurityDescriptorTemplate, pipeServerAccess, pipeClientAccess, sid), nil
}

// Dial connects to the executor's named pipe with the same access par-control
// requests, and without letting the server impersonate the caller.
func Dial(ctx context.Context, address string, timeout time.Duration) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return winio.DialPipeAccessImpLevel(ctx, address, pipeClientAccess, winio.PipeImpLevelIdentification)
}
