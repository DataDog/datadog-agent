// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package packages

import (
	"os"
	"strconv"
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
		Enabled any `yaml:"enabled"`
	} `yaml:"process_manager"`
}

func resolveProcmgrEnabled(lookupEnv func(string) (string, bool), install installEnv, datadogYAML string) (enabled bool, source string) {
	if enabled, ok := procmgrEnabledFromEnv(lookupEnv, procmgrSourceEnvironment); ok {
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

func procmgrEnabledFromEnv(lookupEnv func(string) (string, bool), source string) (bool, bool) {
	raw, ok := lookupEnv(envProcessManagerEnabled)
	if !ok {
		return false, false
	}
	return interpretProcmgrEnv(raw, source)
}

func procmgrEnabledFromInstallScript(install installEnv) (bool, bool) {
	raw, ok := install[envProcessManagerEnabled]
	if !ok {
		return false, false
	}
	return interpretProcmgrEnv(raw, procmgrSourceInstallScript)
}

func interpretProcmgrEnv(raw string, source string) (bool, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return false, false
	}
	enabled := env.ProcessManagerEnabledValue(value)
	if !strings.EqualFold(value, "true") && !strings.EqualFold(value, "false") {
		log.Warnf("%s=%q is neither true nor false (from %s); treating it as %t", envProcessManagerEnabled, value, source, enabled)
	}
	return enabled, true
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
	return procmgrBool(setting.ProcessManager.Enabled, path)
}

func procmgrBool(value any, path string) (bool, bool) {
	if value == nil {
		return false, false
	}
	switch v := value.(type) {
	case bool:
		return v, true
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(v))
		if err != nil {
			log.Warnf("process_manager.enabled in %s is %q, which is not a boolean; ignoring it", path, v)
			return false, false
		}
		return parsed, true
	case int:
		return v != 0, true
	case int64:
		return v != 0, true
	case uint64:
		return v != 0, true
	case float32:
		return v != 0, true
	case float64:
		return v != 0, true
	default:
		log.Warnf("process_manager.enabled in %s has unsupported type %T; ignoring it", path, value)
		return false, false
	}
}
