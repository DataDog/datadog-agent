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

	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config/settings"
	settingshttp "github.com/DataDog/datadog-agent/pkg/config/settings/http"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
	"github.com/DataDog/datadog-agent/pkg/version"
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

// GetVersion returns the version of the agent in a http response json
func GetVersion(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	av, _ := version.Agent()
	j, _ := json.Marshal(av)
	w.Write(j)
}

// NewSettingsClient returns a configured runtime settings client.
func NewSettingsClient() (settings.Client, error) {
	hc := util.GetClient().WithNoVerify().WithTimeout(0).WithResolver().Build()
	return settingshttp.NewClient(hc, fmt.Sprintf("https://%v/agent/config", util.CoreCmd), "agent", settingshttp.NewHTTPClientOptions(util.LeaveConnectionOpen)), nil
}
