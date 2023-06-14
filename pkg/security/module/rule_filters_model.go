// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package module

import (
	"reflect"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

func (e *RuleFilterEvent) Init() {}

func (e *RuleFilterEvent) GetFieldEventType(field eval.Field) (string, error) {
	return "*", nil
}

func (e *RuleFilterEvent) SetFieldValue(field eval.Field, value interface{}) error {
	return &eval.ErrFieldNotFound{Field: field}
}

func (e *RuleFilterEvent) GetFieldType(field eval.Field) (reflect.Kind, error) {
	switch field {
	case "kernel.version.major", "kernel.version.minor", "kernel.version.patch", "kernel.version.abi":
		return reflect.Int, nil
	case "kernel.version.flavor",
		"os.id", "os.platform_id", "os.version_id":
		return reflect.String, nil
	case "os.is_amazon_linux", "os.is_cos", "os.is_debian", "os.is_oracle", "os.is_rhel", "os.is_rhel7",
		"os.is_rhel8", "os.is_sles", "os.is_sles12", "os.is_sles15":
		return reflect.Bool, nil
	}

	return reflect.Invalid, &eval.ErrFieldNotFound{Field: field}
}

func (e *RuleFilterEvent) GetType() string {
	return "*"
}

func (e *RuleFilterEvent) GetTags() []string {
	return []string{}
}

func (m *RuleFilterModel) ValidateField(key string, value eval.FieldValue) error {
	return nil
}

func (m *RuleFilterModel) GetIterator(field eval.Field) (eval.Iterator, error) {
	return nil, &eval.ErrIteratorNotSupported{Field: field}
}
