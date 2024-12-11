// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kfilters

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func init() {
	allCapabilities["create"] = rules.FieldCapabilities{
		{
			Field:       "create.file.name",
			TypeBitmask: eval.ScalarValueType | eval.PatternValueType,
		},
	}
	allCapabilities["rename"] = rules.FieldCapabilities{
		{
			Field:       "rename.file.name",
			TypeBitmask: eval.ScalarValueType | eval.PatternValueType,
		},
	}
	allCapabilities["delete"] = rules.FieldCapabilities{
		{
			Field:       "delete.file.name",
			TypeBitmask: eval.ScalarValueType | eval.PatternValueType,
		},
	}
	allCapabilities["write"] = rules.FieldCapabilities{
		{
			Field:       "write.file.name",
			TypeBitmask: eval.ScalarValueType | eval.PatternValueType,
		},
	}
}
