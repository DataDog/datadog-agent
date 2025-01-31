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

	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config/settings"
	settingshttp "github.com/DataDog/datadog-agent/pkg/config/settings/http"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// GetVersion returns the version of the agent in a http response json
func GetVersion(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	av, _ := version.Agent()
	j, _ := json.Marshal(av)
	w.Write(j)
}

// NewSettingsClient returns a configured runtime settings client.
func NewSettingsClient() (settings.Client, error) {
	ipcAddress, err := pkgconfigsetup.GetIPCAddress(pkgconfigsetup.Datadog())
	if err != nil {
		return nil, err
	}
	hc := util.GetClient(false)
	return settingshttp.NewClient(hc, fmt.Sprintf("https://%v:%v/agent/config", ipcAddress, pkgconfigsetup.Datadog().GetInt("cmd_port")), "agent", settingshttp.NewHTTPClientOptions(util.LeaveConnectionOpen)), nil
}
