// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package procmgrdecision

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cast"
	"go.yaml.in/yaml/v2" // not v3 due to lenient duplicate mapping-key handling
)

// yamlConflictWarning is the warning for a file that sets both forms to different bools.
const yamlConflictWarning = "process_manager.enabled is both true and false"

// SeedFromDatadogYAML reads process_manager.enabled from datadog.yaml, the Windows
// and macOS legacy knob.
//
// A missing file or a file without the setting is not found. A file that cannot be
// read or parsed, or a value that is not a bool, is a *SeedError: such a file may
// still contain an opt-out, so the caller must not fall back to the default.
func SeedFromDatadogYAML(path string) (SeedResult, error) {
	res := SeedResult{Detail: path}
	data, found, err := readSeedFile(path)
	if err != nil {
		return res, &SeedError{Detail: path, Err: err}
	}
	if !found {
		return res, nil
	}
	enabled, set, warning, err := parseDatadogYAML(data)
	if err != nil {
		return res, &SeedError{Detail: path, Err: err}
	}
	if set {
		res.Found = true
		res.Enabled = enabled
		res.Warning = warning
	}
	return res, nil
}

// parseDatadogYAML reads process_manager.enabled. Keys are normalized the same way
// as the Agent config loader: case is ignored, a dotted key is the same setting as
// the nested mapping, and YAML merge keys are applied. A null value does not clear
// the other form. When the two forms are both bools and disagree, the result is
// false with a warning.
func parseDatadogYAML(data []byte) (enabled bool, set bool, warning string, err error) {
	// map[string]interface{} applies YAML merges. MapSlice keeps the "<<:" key
	// literally, which would drop an opt-out inherited through a merge.
	var root map[string]interface{}
	if err := yaml.Unmarshal(data, &root); err != nil {
		return false, false, "", fmt.Errorf("could not parse YAML: %w", err)
	}
	var forms yamlForms
	for key, value := range root {
		switch strings.ToLower(key) {
		case "process_manager.enabled":
			forms.add(yamlFormFromValue(value))
		case "process_manager":
			forms.addSection(value)
		}
	}
	return forms.result()
}

type yamlFormKind int

const (
	yamlFormNull yamlFormKind = iota
	yamlFormBool
	yamlFormBad
)

// yamlForm is one occurrence of the setting.
type yamlForm struct {
	kind    yamlFormKind
	enabled bool
}

// yamlForms accumulates the occurrences of the setting across both forms.
type yamlForms struct {
	sawBool  bool
	enabled  bool
	conflict bool
	bad      bool
}

func (forms *yamlForms) add(value yamlForm) {
	switch value.kind {
	case yamlFormBool:
		if forms.sawBool && forms.enabled != value.enabled {
			forms.conflict = true
		}
		forms.sawBool = true
		forms.enabled = value.enabled
	case yamlFormBad:
		forms.bad = true
	}
}

func (forms yamlForms) result() (enabled bool, set bool, warning string, err error) {
	// A malformed form wins over a valid bool: the malformed one may be the opt-out.
	if forms.bad {
		return false, false, "", errors.New("process_manager.enabled is not a bool")
	}
	if forms.conflict {
		return false, true, yamlConflictWarning, nil
	}
	if forms.sawBool {
		return forms.enabled, true, "", nil
	}
	return false, false, "", nil
}

// yamlFormFromValue classifies one process_manager.enabled value. Null stays unset.
// Any other value is converted with the Agent's bool cast, so a quoted "false" and a
// 0 are false. A value the cast cannot convert is malformed.
func yamlFormFromValue(value interface{}) yamlForm {
	if value == nil {
		return yamlForm{kind: yamlFormNull}
	}
	enabled, err := cast.ToBoolE(value)
	if err != nil {
		return yamlForm{kind: yamlFormBad}
	}
	return yamlForm{kind: yamlFormBool, enabled: enabled}
}

// addSection reads enabled from a process_manager value. A mapping that does not set
// enabled adds nothing, so it does not clear the dotted form. A scalar section is
// malformed.
func (forms *yamlForms) addSection(value interface{}) {
	if value == nil {
		return
	}
	mapping, ok := value.(map[interface{}]interface{})
	if !ok {
		forms.add(yamlForm{kind: yamlFormBad})
		return
	}
	for key, item := range mapping {
		text, ok := key.(string)
		if !ok || strings.ToLower(text) != "enabled" {
			continue
		}
		forms.add(yamlFormFromValue(item))
	}
}
