// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rules holds rules related files
package rules

import (
	"reflect"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

// Init inits the rule filter event
func (e *RuleFilterEvent) Init() {}

// GetFieldEventType returns the event type for the given field
func (e *RuleFilterEvent) GetFieldEventType(_ eval.Field) (string, error) {
	return "*", nil
}

// SetFieldValue sets the value for the given field
func (e *RuleFilterEvent) SetFieldValue(field eval.Field, _ interface{}) error {
	return &eval.ErrFieldNotFound{Field: field}
}

// GetFieldType get the type of the field
func (e *RuleFilterEvent) GetFieldType(field eval.Field) (reflect.Kind, error) {
	switch field {
	case "kernel.version.major", "kernel.version.minor", "kernel.version.patch", "kernel.version.abi":
		return reflect.Int, nil
	case "kernel.version.flavor",
		"os", "os.id", "os.platform_id", "os.version_id", "envs":
		return reflect.String, nil
	case "os.is_amazon_linux", "os.is_cos", "os.is_debian", "os.is_oracle", "os.is_rhel", "os.is_rhel7",
		"os.is_rhel8", "os.is_sles", "os.is_sles12", "os.is_sles15":
		return reflect.Bool, nil
	}

	return reflect.Invalid, &eval.ErrFieldNotFound{Field: field}
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

// GetIterator returns an iterator for the given field
func (m *RuleFilterModel) GetIterator(field eval.Field) (eval.Iterator, error) {
	return nil, &eval.ErrIteratorNotSupported{Field: field}
}
