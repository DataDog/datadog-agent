// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package server

import (
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"

	"github.com/Microsoft/go-winio"
)

const (
	// Buffer sizes for the system probe named pipe.
	// The sizes are advisory, Windows can adjust them, but should be small enough to preserve
	// the non-paged pool.
	namedPipeInputBufferSize  = int32(4096)
	namedPipeOutputBufferSize = int32(4096)

	// DACL template for the system probe named pipe that allows a specific SID.
	// SE_DACL_PROTECTED (P), SE_DACL_AUTO_INHERITED (AI)
	// Allow Administorators (BA), Local System (SY)
	// Allow a custom SID, NO_PROPAGATE_INHERIT_ACE (NP)
	namedPipeSecurityDescriptorTemplate = "D:PAI(A;;FA;;;BA)(A;;FA;;;SY)(A;NP;FRFW;;;%s)"

	// Default DACL for the system probe named pipe.
	// Allow Administorators (BA), Local System (SY)
	namedPipeDefaultSecurityDescriptor = "D:PAI(A;;FA;;;BA)(A;;FA;;;SY)"

	// SID representing Everyone
	everyoneSid = "S-1-1-0"
)

// setupSecurityDescriptor prepares the security descriptor for the system probe named pipe.
func setupSecurityDescriptor() (string, error) {
	// Set up the DACL to allow ddagentuser.
	sid, err := winutil.GetDDAgentUserSID()
	if err != nil {
		return "", fmt.Errorf("failed to get SID for ddagentuser: %s", err)
	}

	sidString := sid.String()

	// Sanity checks
	if len(sidString) == 0 {
		return "", errors.New("failed to get SID string from ddagentuser")
	}

	if sidString == everyoneSid {
		return "", errors.New("ddagentuser as Everyone is not supported")
	}

	sd, err := formatSecurityDescriptorWithSid(sidString)
	if err != nil {
		return "", fmt.Errorf("invalid SID from ddagentuser: %s", err)
	}

	log.Debugf("named pipe DACL prepared with ddagentuser %s", sidString)
	return sd, nil
}

// formatSecurityDescriptorWithSid creates a security descriptor string for the system probe
// named pipe that allows a set of default users and the specified SID.
func formatSecurityDescriptorWithSid(sidString string) (string, error) {
	// Sanity check
	if !strings.HasPrefix(sidString, "S-") {
		return "", fmt.Errorf("invalid SID %s", sidString)
	}
	return fmt.Sprintf(namedPipeSecurityDescriptorTemplate, sidString), nil
}

// NewListener sets up a named pipe listener for the system probe service.
func NewListener(namedPipeName string) (net.Listener, error) {
	sd, err := setupSecurityDescriptor()
	if err != nil {
		log.Errorf("failed to setup security descriptor, ddagentuser is denied: %s", err)

		// The default security descriptor does not include ddagentuser.
		// Queries from the DD agent will fail.
		sd = namedPipeDefaultSecurityDescriptor
	}

	return newListenerWithSecurityDescriptor(namedPipeName, sd)
}

// newListenerWithSecurityDescriptor sets up a named pipe listener with a security descriptor.
func newListenerWithSecurityDescriptor(namedPipeName string, securityDescriptor string) (net.Listener, error) {
	config := winio.PipeConfig{
		SecurityDescriptor: securityDescriptor,
		InputBufferSize:    namedPipeInputBufferSize,
		OutputBufferSize:   namedPipeOutputBufferSize,
	}

	// winio specifies virtually unlimited number of named pipe instances but is limited by
	// the nonpaged pool.
	namedPipe, err := winio.ListenPipe(namedPipeName, &config)
	if err != nil {
		return nil, fmt.Errorf("named pipe listener %q: %s", namedPipeName, err)
	}

	log.Infof("named pipe %s ready", namedPipeName)

	return namedPipe, nil
}
