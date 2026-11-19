// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package packages

import (
	"os"
	"strings"

	"go.yaml.in/yaml/v2" // not v3 due to lenient duplicate mapping-key handling

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	procmgrSourceEnvironment   = "environment"
	procmgrSourceInstallScript = "install script"
	procmgrSourceDatadogYAML   = "datadog.yaml"
	procmgrSourceDefault       = "default"

	envProcessManagerEnabled = "DD_PROCESS_MANAGER_ENABLED"
)

// procmgrEnabled reports whether this install runs dd-procmgrd, and where that came from.
var procmgrEnabled = resolveProcmgrEnabled

type procmgrSetting struct {
	ProcessManager struct {
		Enabled *bool `yaml:"enabled"`
	} `yaml:"process_manager"`
}

func resolveProcmgrEnabled(lookupEnv func(string) (string, bool), install installEnv, datadogYAML string) (enabled bool, source string) {
	if enabled, ok := procmgrEnabledFromEnv(lookupEnv); ok {
		return enabled, procmgrSourceEnvironment
	}
	if enabled, ok := procmgrEnabledFromInstallScript(install); ok {
		return enabled, procmgrSourceInstallScript
	}
	if enabled, ok := procmgrEnabledFromYAML(datadogYAML); ok {
		return enabled, procmgrSourceDatadogYAML
	}
	return true, procmgrSourceDefault
}

func procmgrEnabledFromEnv(lookupEnv func(string) (string, bool)) (bool, bool) {
	raw, ok := lookupEnv(envProcessManagerEnabled)
	if !ok {
		return false, false
	}
	return interpretProcmgrEnv(raw)
}

func procmgrEnabledFromInstallScript(install installEnv) (bool, bool) {
	raw, ok := install[envProcessManagerEnabled]
	if !ok {
		return false, false
	}
	return interpretProcmgrEnv(raw)
}

func interpretProcmgrEnv(raw string) (bool, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return false, false
	}
	return env.ProcessManagerEnabledValue(value), true
}

func procmgrEnabledFromYAML(path string) (bool, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Warnf("could not read %s: %v", path, err)
		}
		return false, false
	}
	var setting procmgrSetting
	if err := yaml.Unmarshal(data, &setting); err != nil {
		log.Warnf("could not parse %s: %v", path, err)
		return false, false
	}
	if setting.ProcessManager.Enabled == nil {
		return false, false
	}
	return *setting.ProcessManager.Enabled, true
}
