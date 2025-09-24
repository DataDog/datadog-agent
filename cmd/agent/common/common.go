// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package common provides a set of common symbols needed by different packages,
// to avoid circular dependencies.
package common

import (
	"fmt"
	"path/filepath"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipchttp "github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers"
	"github.com/DataDog/datadog-agent/pkg/config/settings"
	settingshttp "github.com/DataDog/datadog-agent/pkg/config/settings/http"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
)

// GetPythonPaths returns the paths (in order of precedence) from where the agent
// should load python modules and checks
func GetPythonPaths() []string {
	// wheels install in default site - already in sys.path; takes precedence over any additional location
	return []string{
		defaultpaths.GetDistPath(),                               // common modules are shipped in the dist path directly or under the "checks/" sub-dir
		defaultpaths.PyChecksPath,                                // integrations-core legacy checks
		filepath.Join(defaultpaths.GetDistPath(), "checks.d"),    // custom checks in the "checks.d/" sub-dir of the dist path
		pkgconfigsetup.Datadog().GetString("additional_checksd"), // custom checks, least precedent check location
	}
}

// NewSettingsClient returns a configured runtime settings client.
func NewSettingsClient(client ipc.HTTPClient) (settings.Client, error) {
	ipcAddress, err := pkgconfigsetup.GetIPCAddress(pkgconfigsetup.Datadog())
	if err != nil {
		return nil, err
	}
	return settingshttp.NewHTTPSClient(client, fmt.Sprintf("https://%v:%v/agent/config", ipcAddress, pkgconfigsetup.Datadog().GetInt("cmd_port")), "agent", ipchttp.WithLeaveConnectionOpen), nil
}
