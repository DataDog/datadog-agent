// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package server

import (
	"fmt"
	"net"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/Microsoft/go-winio"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
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

var (
	namedPipeSecurityDescriptor = namedPipeDefaultSecurityDescriptor
)

// getDDAgentUser returns the domain name of the ddagentuser configured at installation time
func getDDAgentUser() (string, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Datadog\Datadog Agent`, registry.QUERY_VALUE)
	if err != nil {
		return "", fmt.Errorf("failed to open installer registry: %s", err)
	}
	defer k.Close()

	user, _, err := k.GetStringValue("installedUser")
	if err != nil {
		return "", fmt.Errorf("failed to read installedUser in registry: %s", err)
	}

	domain, _, err := k.GetStringValue("installedDomain")
	if err != nil {
		return "", fmt.Errorf("failed to read installedDomain in registry: %s", err)
	}

	if domain != "" {
		user = domain + `\` + user
	}

	return user, nil
}

// SetupPermissions prepares permissions prior to starting the system probe server.
func SetupPermissions() {
	// Set up the DACL for the System Probe named pipe to allow ddagentuser.
	user, err := getDDAgentUser()
	if err != nil {
		log.Errorf("failed to get ddagentuser: %s", err)
		return
	}

	sid, _, _, err := windows.LookupSID("", user)
	if err != nil {
		log.Errorf("failed to get SID for ddagentuser %s: %s", user, err)
		return
	}

	sidString := sid.String()

	// Sanity checks
	if len(sidString) == 0 {
		log.Errorf("failed to get SID string from ddagentuser %s", user)
		return
	}

	if sidString == everyoneSid {
		log.Error("ddagentuser as Everyone is not supported")
		return
	}

	sd, err := FormatSecurityDescriptorWithSid(sidString)
	if err != nil {
		log.Errorf("Invalid SID from ddagentuser: %s", sidString)
		return
	}

	log.Debugf("named pipe DACL prepared with ddagentuser %s", user)
	namedPipeSecurityDescriptor = sd
}

// FormatSecurityDescriptorWithSid creates a security descriptor string for the system probe
// named pipe that allows a set of default users and the specified SID.
func FormatSecurityDescriptorWithSid(sidString string) (string, error) {
	// Sanity check
	if !strings.HasPrefix(sidString, "S-") {
		return "", fmt.Errorf("Invalid SID %s", sidString)
	}
	return fmt.Sprintf(namedPipeSecurityDescriptorTemplate, sidString), nil
}

// NewListener sets up a named pipe listener for the system probe service.
func NewListener(namedPipeName string) (net.Listener, error) {
	// The DACL in the security descriptor must allow ddagentuser and Administrators.
	return NewListenerWithSecurityDescriptor(namedPipeName, namedPipeSecurityDescriptor)
}

// NewListenerWithSecurityDescriptor sets up a named pipe listener with a security descriptor.
func NewListenerWithSecurityDescriptor(namedPipeName string, securityDescriptor string) (net.Listener, error) {
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
