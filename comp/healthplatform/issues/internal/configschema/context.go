// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package configschema shares configuration issue details between Agent and system-probe checks.
package configschema

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/qri-io/jsonpointer"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/schema"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

const (
	contextKeyConfigPath = "config_path"
	contextKeyErrorCount = "error_count"
	contextKeyViolations = "violations"
)

func contextErrorKey(i int) string { return "error." + strconv.Itoa(i) }

// ViolationPayload contains types and defaults, never the configured value.
type ViolationPayload struct {
	Path          string   `json:"path"`
	ActualType    string   `json:"actual_type"`
	ExpectedTypes []string `json:"expected_types"`
	DefaultStatus string   `json:"default_status"`
	DefaultValue  any      `json:"default_value,omitempty"`
}

// BuildContext builds issue details from scrubbed paths, types, and registered defaults.
func BuildContext(cfg model.Reader, configPath string, violations []schema.Violation) map[string]string {
	ctx := map[string]string{
		contextKeyConfigPath: configPath,
		contextKeyErrorCount: strconv.Itoa(len(violations)),
	}
	payloads := make([]ViolationPayload, 0, len(violations))
	for i, violation := range violations {
		path := scrubViolationPath(violation.Path)
		// Raw validator messages can expose configured values or credentials in paths.
		// Build messages from scrubbed paths and type names instead.
		ctx[contextErrorKey(i)] = fmt.Sprintf("at '%s': configuration does not match schema", path)
		if violation.ActualType == "" || len(violation.ExpectedTypes) == 0 {
			continue
		}
		ctx[contextErrorKey(i)] = fmt.Sprintf("at '%s': got %s, want %s", path, violation.ActualType, strings.Join(violation.ExpectedTypes, " or "))
		defaultStatus, defaultValue := resolveDefault(cfg, violation.Path)
		payloads = append(payloads, ViolationPayload{
			Path:          path,
			ActualType:    violation.ActualType,
			ExpectedTypes: violation.ExpectedTypes,
			DefaultStatus: defaultStatus,
			DefaultValue:  defaultValue,
		})
	}
	if len(payloads) == len(violations) {
		if encoded, err := json.Marshal(payloads); err == nil {
			ctx[contextKeyViolations] = string(encoded)
		}
	}
	return ctx
}

func scrubViolationPath(path string) string {
	pointer, err := jsonpointer.Parse(path)
	if err != nil {
		return ""
	}
	for i, token := range pointer {
		pointer[i], err = scrubber.ScrubString(token)
		if err != nil {
			return ""
		}
	}
	return pointer.String()
}

func resolveDefault(cfg model.Reader, pointerPath string) (string, any) {
	pointer, err := jsonpointer.Parse(pointerPath)
	if err != nil || pointer.IsEmpty() {
		return "unknown", nil
	}
	for _, token := range pointer {
		if token == "" || strings.Contains(token, ".") {
			return "unknown", nil
		}
	}
	key := strings.Join(pointer, ".")
	if !cfg.IsSetting(key) {
		return "unknown", nil
	}

	for _, valueWithSource := range cfg.GetAllSources(key) {
		if valueWithSource.Source != model.SourceDefault {
			continue
		}
		switch value := valueWithSource.Value.(type) {
		case nil:
			return "none", nil
		case time.Duration:
			return "known", value.String()
		default:
			return "known", value
		}
	}
	return "none", nil
}
