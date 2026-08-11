// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package daemon

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Microsoft/go-winio"

	daemonstatus "github.com/DataDog/datadog-agent/pkg/fleet/daemon/status"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

const (
	// DACL template for the status named pipe, allowing ddagentuser to read it.
	// SE_DACL_PROTECTED (P), SE_DACL_AUTO_INHERITED (AI)
	// Allow Administrators (BA), Local System (SY)
	// Allow a custom SID, NO_PROPAGATE_INHERIT_ACE (NP)
	//
	// Same descriptor as system-probe's named pipe
	// (pkg/system-probe/api/server/listener_windows.go): the local API pipe stays
	// SYSTEM + Administrators only because its routes are privileged mutations.
	statusPipeSecurityDescriptorTemplate = "D:PAI(A;;FA;;;BA)(A;;FA;;;SY)(A;NP;FRFW;;;%s)"

	// Default DACL for the status named pipe, used when the ddagentuser SID cannot
	// be resolved. The Agent will not be able to read the status, but the daemon
	// must still start.
	statusPipeDefaultSecurityDescriptor = "D:PAI(A;;FA;;;BA)(A;;FA;;;SY)"

	// SID representing Everyone
	everyoneSid = "S-1-1-0"
)

// setupStatusSecurityDescriptor prepares the security descriptor for the status named pipe.
func setupStatusSecurityDescriptor() (string, error) {
	sid, err := winutil.GetDDAgentUserSID()
	if err != nil {
		return "", fmt.Errorf("failed to get SID for ddagentuser: %w", err)
	}

	sidString := sid.String()

	// Sanity checks
	if len(sidString) == 0 {
		return "", errors.New("failed to get SID string from ddagentuser")
	}
	if sidString == everyoneSid {
		return "", errors.New("ddagentuser as Everyone is not supported")
	}
	if !strings.HasPrefix(sidString, "S-") {
		return "", fmt.Errorf("invalid SID %s", sidString)
	}

	log.Debugf("installer status named pipe DACL prepared with ddagentuser %s", sidString)
	return fmt.Sprintf(statusPipeSecurityDescriptorTemplate, sidString), nil
}

// NewStatusAPI returns a new StatusAPI.
func NewStatusAPI(daemon Daemon) (StatusAPI, error) {
	// Prevent daemon from running in insecure directories
	if err := paths.IsInstallerDataDirSecure(); err != nil {
		return nil, err
	}
	return newStatusAPI(daemon, daemonstatus.Address())
}

func newStatusAPI(daemon statusProvider, namedPipePath string) (StatusAPI, error) {
	sd, err := setupStatusSecurityDescriptor()
	if err != nil {
		// The default security descriptor does not include ddagentuser: the Agent's
		// installer metadata will report the installer as unreachable, but the daemon
		// itself keeps working.
		log.Errorf("failed to setup installer status security descriptor, ddagentuser is denied: %s", err)
		sd = statusPipeDefaultSecurityDescriptor
	}

	listener, err := winio.ListenPipe(namedPipePath, &winio.PipeConfig{
		SecurityDescriptor: sd,
		MessageMode:        false,
	})
	if err != nil {
		return nil, err
	}

	return &statusAPIImpl{
		server:   &http.Server{},
		listener: listener,
		daemon:   daemon,
	}, nil
}
