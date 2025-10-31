// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package hostname provides utilities to detect the hostname of the host.
package hostname

import (
	"context"
	"fmt"
	"os"
	"strings"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/hostname/validate"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// fromHostnameFileDLL attempts to resolve the hostname via a file using the DLL implementation.
// This mirrors fromHostnameFile config behavior but is handled by the Rust side.
func fromHostnameFileDLL(ctx context.Context, hostnameFilepath string) (string, error) {
	return dllResolveHostname("file", hostnameFilepath)
}

// fromHostnameFileLocalGo attempts to resolve the hostname via a file using the local Go implementation.
func fromHostnameFileLocalGo(ctx context.Context, hostnameFilepath string) (string, error) {
	fileContent, err := os.ReadFile(hostnameFilepath)
	if err != nil {
		return "", fmt.Errorf("Could not read hostname from %s: %v", hostnameFilepath, err)
	}

	hostname := strings.TrimSpace(string(fileContent))

	err = validate.ValidHostname(hostname)
	if err != nil {
		return "", err
	}
	warnIfNotCanonicalHostname(ctx, hostname)
	return hostname, nil
}

func fromHostnameFile(ctx context.Context, _ string) (string, error) {
	hostnameFilepath := pkgconfigsetup.Datadog().GetString("hostname_file")
	if hostnameFilepath == "" {
		return "", fmt.Errorf("'hostname_file' configuration is not enabled")
	}

	// Get the configuration around what implementation to use
	useDLLHostnameFileImpl := pkgconfigsetup.Datadog().GetBool("use_dll_hostname_resolution.file.enabled")
	log.Infof("Use DLL hostname file implementation: %v", useDLLHostnameFileImpl)

	// Get the hostname from the local Go implementation
	localGoHostname, localGoHostnameErr := fromHostnameFileLocalGo(ctx, hostnameFilepath)
	if localGoHostnameErr != nil {
		log.Warnf("Error getting hostname from file via local Go: %s", localGoHostnameErr)
	}

	// Get the hostname from the DLL implementation
	dllHostname, dllHostnameErr := fromHostnameFileDLL(ctx, hostnameFilepath)
	if dllHostnameErr != nil {
		log.Warnf("Error getting hostname from file via DLL: %s", dllHostnameErr)
	}

	// Plan ahead of time on what we want to return
	outHostname, outHostnameErr := localGoHostname, localGoHostnameErr
	if useDLLHostnameFileImpl {
		outHostname, outHostnameErr = dllHostname, dllHostnameErr
	}

	// Both implementations had problems
	if dllHostnameErr != nil && localGoHostnameErr != nil {
		log.Errorf("Hostname file detected through DLL and local Go both had problems!", dllHostnameErr, localGoHostnameErr)
		return outHostname, outHostnameErr
	}

	// One implementation had a problem, the other didn't
	if (dllHostnameErr != nil && localGoHostnameErr == nil) || (dllHostnameErr == nil && localGoHostnameErr != nil) {
		log.Errorf("Hostname file detected through DLL and local Go had different problems! (%v, %v)", dllHostnameErr, localGoHostnameErr)
	}

	// Hostnames are different
	if !(dllHostname == localGoHostname) {
		log.Warnf("Hostame detected through file via DLL and local Go are different! (local: %s, DLL: %s)", localGoHostname, dllHostname)
		return outHostname, outHostnameErr
	}

	// TODO: This should be a debug log
	log.Warnf("Hostame detected through file via DLL and local Go are the same! (%s)", outHostname)
	return outHostname, outHostnameErr
}
