// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package packages

import (
	"os"
	"strings"

	"github.com/spf13/cast"
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

// procmgrEnabled is indirected so the hook tests do not depend on the host's environment or datadog.yaml.
var procmgrEnabled = resolveProcmgrEnabled

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

// procmgrEnabledFromYAML reads process_manager.enabled. Keys are normalized the
// same way as the Agent config loader: case is ignored, a dotted key is the same
// setting as the nested mapping, and YAML merge keys are applied. A null value
// does not clear the other form. When the two forms are both bools and disagree,
// the installer keeps false and logs a warning.
func procmgrEnabledFromYAML(path string) (bool, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Warnf("could not read %s: %v", path, err)
		}
		return false, false
	}
	// map[string]interface{} applies YAML merges. MapSlice keeps the "<<:" key
	// literally, which would drop an opt-out inherited through a merge.
	var root map[string]interface{}
	if err := yaml.Unmarshal(data, &root); err != nil {
		log.Warnf("could not parse %s: %v", path, err)
		return false, false
	}
	enabled, found, reason := procmgrEnabledInDocument(root)
	if reason != "" {
		log.Warnf("could not parse %s: %s", path, reason)
	}
	if !found {
		return false, false
	}
	return enabled, true
}

type procmgrFormValue struct {
	kind    procmgrFormKind
	enabled bool
}

type procmgrFormKind int

const (
	procmgrFormNull procmgrFormKind = iota
	procmgrFormBool
	procmgrFormBad
)

type procmgrForms struct {
	sawBool  bool
	enabled  bool
	conflict bool
	bad      bool
}

func procmgrEnabledInDocument(root map[string]interface{}) (enabled bool, found bool, reason string) {
	var forms procmgrForms
	for key, value := range root {
		switch strings.ToLower(key) {
		case "process_manager.enabled":
			forms.add(procmgrFormFromValue(value))
		case "process_manager":
			forms.addSection(value)
		}
	}
	return forms.result()
}

func (forms *procmgrForms) add(value procmgrFormValue) {
	switch value.kind {
	case procmgrFormBool:
		if forms.sawBool && forms.enabled != value.enabled {
			forms.conflict = true
		}
		forms.sawBool = true
		forms.enabled = value.enabled
	case procmgrFormBad:
		forms.bad = true
	}
}

func (forms procmgrForms) result() (enabled bool, found bool, reason string) {
	if forms.conflict {
		return false, true, "process_manager.enabled is both true and false"
	}
	if forms.sawBool {
		return forms.enabled, true, ""
	}
	if forms.bad {
		return false, false, "process_manager.enabled is not a bool"
	}
	return false, false, ""
}

// procmgrFormFromValue classifies one process_manager.enabled value. Null stays
// unset. Any other value is converted with the Agent's bool cast, so a quoted
// "false" and a 0 are false. A value the cast cannot convert is malformed.
func procmgrFormFromValue(value interface{}) procmgrFormValue {
	if value == nil {
		return procmgrFormValue{kind: procmgrFormNull}
	}
	enabled, err := cast.ToBoolE(value)
	if err != nil {
		return procmgrFormValue{kind: procmgrFormBad}
	}
	return procmgrFormValue{kind: procmgrFormBool, enabled: enabled}
}

// addSection reads enabled from a process_manager value. A mapping that does not
// set enabled adds nothing, so it does not clear the dotted form. A scalar
// section is malformed.
func (forms *procmgrForms) addSection(value interface{}) {
	if value == nil {
		return
	}
	mapping, ok := value.(map[interface{}]interface{})
	if !ok {
		forms.add(procmgrFormValue{kind: procmgrFormBad})
		return
	}
	for key, item := range mapping {
		text, ok := key.(string)
		if !ok || strings.ToLower(text) != "enabled" {
			continue
		}
		forms.add(procmgrFormFromValue(item))
	}
}
