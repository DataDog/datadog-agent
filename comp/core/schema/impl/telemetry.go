// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package schemaimpl

import (
	"strings"

	"github.com/qri-io/jsonpointer"

	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/installinfo"
)

const unknownTag = "unknown"

// installMethodGetter is a variable so tests don't depend on the install info of the host
var installMethodGetter = func(config model.Reader) string {
	info, err := installinfo.Get(config)
	if err != nil || info.Tool == "" {
		return unknownTag
	}
	return info.Tool
}

// violationTelemetry reports schema violations through the config.schema_violations gauge, exported by COAT. Its tags
// must never carry the configured values: they only hold schema-defined names and generic reasons.
type violationTelemetry struct {
	violations telemetry.Gauge
}

func newViolationTelemetry(tlm telemetry.Component) *violationTelemetry {
	return &violationTelemetry{
		violations: tlm.NewGauge(
			"config",
			"schema_violations",
			[]string{"config", "setting", "source", "install_method", "error"},
			"Number of configuration settings violating the configuration schema",
		),
	}
}

// report replaces the series of the configuration following schemaID with violations
func (t *violationTelemetry) report(config model.Reader, schemaID model.SchemaID, violations []*model.Violation) {
	configTag := configTag(schemaID)
	t.violations.DeletePartialMatch(map[string]string{"config": configTag})
	if len(violations) == 0 {
		return
	}

	installMethod := installMethodGetter(config)
	for _, violation := range violations {
		setting, source := unknownTag, unknownTag
		if violation.SettingKey != "" {
			setting = violation.SettingKey
			source = string(config.GetSource(violation.SettingKey))
		} else if section := knownSection(config, violation.Path); section != "" {
			setting = section
		}
		t.violations.Inc(configTag, setting, source, installMethod, violation.Reason)
	}
}

// clear removes the series of the configuration following schemaID
func (t *violationTelemetry) clear(schemaID model.SchemaID) {
	t.violations.DeletePartialMatch(map[string]string{"config": configTag(schemaID)})
}

func configTag(schemaID model.SchemaID) string {
	switch schemaID {
	case model.CoreSchema:
		return "datadog"
	case model.SystemProbeSchema:
		return "system-probe"
	}
	return string(schemaID)
}

// knownSection returns the longest prefix of path that the configuration knows. A violation without setting is either
// located on a section (ie: unknown properties) or on an unknown key: the keys of the path past the known prefix are
// chosen by the user and must not be reported.
func knownSection(config model.Reader, path string) string {
	pointer, err := jsonpointer.Parse(path)
	if err != nil {
		return ""
	}
	section := ""
	for i := range pointer {
		key := strings.Join(pointer[:i+1], ".")
		if !config.IsKnown(key) {
			break
		}
		section = key
	}
	return section
}
