// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package common provides a set of common symbols needed by different packages,
// to avoid circular dependencies.
package common

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/DataDog/datadog-agent/cmd/agent/common/path"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/settings"
	settingshttp "github.com/DataDog/datadog-agent/pkg/config/settings/http"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// GetPythonPaths returns the paths (in order of precedence) from where the agent
// should load python modules and checks
func GetPythonPaths() []string {
	// wheels install in default site - already in sys.path; takes precedence over any additional location
	return []string{
		path.GetDistPath(), // common modules are shipped in the dist path directly or under the "checks/" sub-dir
		path.PyChecksPath,  // integrations-core legacy checks
		filepath.Join(path.GetDistPath(), "checks.d"),  // custom checks in the "checks.d/" sub-dir of the dist path
		config.Datadog.GetString("additional_checksd"), // custom checks, least precedent check location
	}
}

// GetVersion returns the version of the agent in a http response json
func GetVersion(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	av, _ := version.Agent()
	j, _ := json.Marshal(av)
	w.Write(j)
}

// NewSettingsClient returns a configured runtime settings client.
func NewSettingsClient() (settings.Client, error) {
	ipcAddress, err := config.GetIPCAddress()
	if err != nil {
		return nil, err
	}
	hc := util.GetClient(false)
	return settingshttp.NewClient(hc, fmt.Sprintf("https://%v:%v/agent/config", ipcAddress, config.Datadog.GetInt("cmd_port")), "agent", settingshttp.NewHTTPClientOptions(util.LeaveConnectionOpen)), nil
}
