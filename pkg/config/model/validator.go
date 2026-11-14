// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package model

import (
	"fmt"
	"strings"
	"sync/atomic"
	"unicode"
)

// Violation is a schema validation error. It carries structured data so consumers don't have to parse the
// human-readable Message. It holds an atomic value and must only be handled through pointers.
type Violation struct {
	Message       string
	Path          string
	SettingKey    string
	ActualType    string
	ExpectedTypes []string
	// Reason describes the violation using only the schema (expected types, allowed values, bounds...). It never
	// contains the configured value, so unlike Message it's safe to report as telemetry.
	Reason   string
	reverted atomic.Bool
}

// MarkAsReverted marks the violation as reverted: its setting was reverted to its default
func (v *Violation) MarkAsReverted() {
	v.reverted.Store(true)
}

// WasReverted returns whether the setting of the violation was reverted to its default
func (v *Violation) WasReverted() bool {
	return v.reverted.Load()
}

// Warning returns a human-readable description of the violation
func (v *Violation) Warning() string {
	message := inlineCode(v.Message)
	if v.SettingKey == "" {
		return "invalid configuration: " + message
	}
	if v.WasReverted() {
		return fmt.Sprintf("invalid value for '%s', reverted to default. Error: %s", v.SettingKey, message)
	}
	return fmt.Sprintf("invalid value for '%s'. Error: %s", v.SettingKey, message)
}

// inlineCode replaces control characters with spaces and wraps text as Markdown code.
// Its fence is longer than any backtick run so the text cannot close it.
func inlineCode(text string) string {
	text = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, text)
	fenceLength := 1
	for _, ticks := range strings.FieldsFunc(text, func(r rune) bool { return r != '`' }) {
		fenceLength = max(fenceLength, len(ticks)+1)
	}
	fence := strings.Repeat("`", fenceLength)
	if strings.HasPrefix(text, "`") || strings.HasSuffix(text, "`") {
		text = " " + text + " "
	}
	return fence + text + fence
}

// SchemaValidator validates a configuration against its schema. It is implemented by the schema component
// (comp/core/schema), and declared here so the config packages don't depend on it.
type SchemaValidator interface {
	// ValidateConfig validates the settings of config against its schema (see Reader.GetSchemaID) and returns the
	// individual violations. It must be called once the configuration file and environment variables are loaded,
	// before secrets are resolved and before the Agent changes any setting at runtime.
	ValidateConfig(config Reader) ([]*Violation, error)
}
