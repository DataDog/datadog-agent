// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package filtermodel holds rules related files
package filtermodel

import (
	"reflect"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/utils/hostnameutils"
)

// RuleFilterEventConfig holds the config used by the rule filter event
type RuleFilterEventConfig struct {
	COREEnabled bool
	Origin      string
}

// Init inits the rule filter event
func (e *RuleFilterEvent) Init() {}

// SetFieldValue sets the value for the given field
func (e *RuleFilterEvent) SetFieldValue(field eval.Field, _ interface{}) error {
	return &eval.ErrFieldNotFound{Field: field}
}

// GetFieldMetadata get the type of the field
func (e *RuleFilterEvent) GetFieldMetadata(field eval.Field) (eval.Field, reflect.Kind, string, error) {
	switch field {
	case "kernel.version.major", "kernel.version.minor", "kernel.version.patch", "kernel.version.abi":
		return "*", reflect.Int, "int", nil
	case "kernel.version.flavor",
		"os", "os.id", "os.platform_id", "os.version_id", "envs", "origin", "hostname":
		return "*", reflect.String, "string", nil
	case "os.is_amazon_linux", "os.is_cos", "os.is_debian", "os.is_oracle", "os.is_rhel", "os.is_rhel7",
		"os.is_rhel8", "os.is_sles", "os.is_sles12", "os.is_sles15", "kernel.core.enabled":
		return "*", reflect.Bool, "bool", nil
	}

	return "", reflect.Invalid, "", &eval.ErrFieldNotFound{Field: field}
}

// GetType returns the type for this event
func (e *RuleFilterEvent) GetType() string {
	return "*"
}

// GetTags returns the tags for this event
func (e *RuleFilterEvent) GetTags() []string {
	return []string{}
}

// ValidateField returns whether the value use against the field is valid
func (m *RuleFilterModel) ValidateField(_ string, _ eval.FieldValue) error {
	return nil
}

// GetFieldRestrictions returns the field event type restrictions
func (m *RuleFilterModel) GetFieldRestrictions(_ eval.Field) []eval.EventType {
	return nil
}

func getHostname(ipcComp ipc.Component) string {
	hostname, err := hostnameutils.GetHostname(ipcComp)
	if err != nil || hostname == "" {
		hostname = "unknown"
	}
	return hostname
}
