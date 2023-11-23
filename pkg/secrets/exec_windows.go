// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018-present Datadog, Inc.

//go:build secrets && windows

package secrets

import (
	"context"
	"fmt"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc/mgr"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

const ddAgentServiceName = "datadogagent"

// commandContext sets up an exec.Cmd for running with a context
func commandContext(ctx context.Context, name string, arg ...string) (*exec.Cmd, func(), error) {
	cmd := exec.CommandContext(ctx, name, arg...)
	done := func() {}
	localSystem, err := getLocalSystemSID()
	if err != nil {
		return nil, nil, fmt.Errorf("could not query Local System SID: %s", err)
	}
	defer windows.FreeSid(localSystem)

	currentUser, err := winutil.GetSidFromUser()
	if err != nil {
		return nil, nil, fmt.Errorf("could not get SID for current user: %s", err)
	}

	// If we are running as Local System we need to "sandbox" execution to "ddagentuser"
	if currentUser.Equals(localSystem) {
		// Retrieve token from the running Datadog Agent service
		token, err := getDDAgentServiceToken()
		if err != nil {
			return nil, nil, err
		}

		done = func() {
			defer windows.CloseHandle(windows.Handle(token))
		}

		// Configure the token to run with
		cmd.SysProcAttr = &syscall.SysProcAttr{Token: syscall.Token(token)}
	}

	return cmd, done, nil
}

// getDDAgentServiceToken retrieves token from the running Datadog Agent service
func getDDAgentServiceToken() (windows.Token, error) {
	var token, duplicatedToken windows.Token
	pid, err := getServicePid(ddAgentServiceName)
	if err != nil {
		return windows.Token(0), fmt.Errorf("could not get pid of %s service: %s", ddAgentServiceName, err)
	}

	procHandle, err := windows.OpenProcess(windows.PROCESS_QUERY_INFORMATION, false, pid)
	if err != nil {
		return windows.Token(0), fmt.Errorf("could not get pid of %s service: %s", ddAgentServiceName, err)
	}

	defer windows.CloseHandle(procHandle)

	if err = windows.OpenProcessToken(procHandle, windows.TOKEN_ALL_ACCESS, &token); err != nil {
		return windows.Token(0), err
	}

	defer windows.CloseHandle(windows.Handle(token))

	if err := windows.DuplicateTokenEx(token, windows.MAXIMUM_ALLOWED, nil, windows.SecurityDelegation, windows.TokenPrimary, &duplicatedToken); err != nil {
		return windows.Token(0), fmt.Errorf("error duplicating %s service token: %s", ddAgentServiceName, err)
	}

	return duplicatedToken, nil
}

// getServicePid gets the PID of a running service
func getServicePid(serviceName string) (uint32, error) {
	h, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return 0, fmt.Errorf("could not connect to SCM: %s", err)
	}

	m := &mgr.Mgr{Handle: h}
	defer m.Disconnect()

	hSvc, err := windows.OpenService(m.Handle, syscall.StringToUTF16Ptr(serviceName),
		windows.SERVICE_QUERY_STATUS)
	if err != nil {
		return 0, fmt.Errorf("could not access service %s: %v", serviceName, err)
	}

	service := &mgr.Service{Name: serviceName, Handle: hSvc}
	defer service.Close()
	status, err := service.Query()
	if err != nil {
		return 0, fmt.Errorf("could not query service %s: %v", serviceName, err)
	}
	return status.ProcessId, nil
}
