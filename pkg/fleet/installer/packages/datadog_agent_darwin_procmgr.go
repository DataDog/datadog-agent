// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package packages

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cast"
	"go.yaml.in/yaml/v2" // not v3 due to lenient duplicate mapping-key handling

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	procmgrSourceEnvironment      = "environment"
	procmgrSourceInstallScript    = "install script"
	procmgrSourceDatadogYAML      = "datadog.yaml"
	procmgrSourceSavedDatadogYAML = "saved datadog.yaml"
	procmgrSourceDefault          = "default"

	envProcessManagerEnabled = "DD_PROCESS_MANAGER_ENABLED"
)

// procmgrEnabled is indirected so the hook tests do not depend on the host's environment or datadog.yaml.
var procmgrEnabled = resolveProcmgrEnabled

func resolveProcmgrEnabled(lookupEnv func(string) (string, bool), install installEnv, datadogYAML string) (enabled bool, source string, err error) {
	if enabled, ok := procmgrEnabledFromEnv(lookupEnv); ok {
		return enabled, procmgrSourceEnvironment, nil
	}
	if enabled, ok := procmgrEnabledFromInstallScript(install); ok {
		return enabled, procmgrSourceInstallScript, nil
	}
	enabled, found, err := procmgrEnabledFromYAML(datadogYAML)
	if err != nil {
		// Fail closed: a file that cannot be read or parsed, or whose value is not
		// a bool, may still contain an opt-out.
		return false, procmgrSourceDatadogYAML, err
	}
	if found {
		return enabled, procmgrSourceDatadogYAML, nil
	}
	return true, procmgrSourceDefault, nil
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
// the installer keeps false and logs a warning. A missing file is an absent
// setting. A file that cannot be read or parsed, or a value that is not a bool,
// is an error, so the caller can keep the process manager off.
func procmgrEnabledFromYAML(path string) (enabled bool, found bool, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, false, nil
		}
		return false, false, fmt.Errorf("could not read %s: %w", path, err)
	}
	// map[string]interface{} applies YAML merges. MapSlice keeps the "<<:" key
	// literally, which would drop an opt-out inherited through a merge.
	var root map[string]interface{}
	if err := yaml.Unmarshal(data, &root); err != nil {
		return false, false, fmt.Errorf("could not parse %s: %w", path, err)
	}
	enabled, found, reason := procmgrEnabledInDocument(root)
	if reason != "" && !found {
		return false, false, fmt.Errorf("could not parse %s: %s", path, reason)
	}
	if reason != "" {
		log.Warnf("could not parse %s: %s", path, reason)
	}
	if !found {
		return false, false, nil
	}
	return enabled, true, nil
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
	// A malformed form wins over a valid bool: the malformed one may be the opt-out.
	if forms.bad {
		return false, false, "process_manager.enabled is not a bool"
	}
	if forms.conflict {
		return false, true, "process_manager.enabled is both true and false"
	}
	if forms.sawBool {
		return forms.enabled, true, ""
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
