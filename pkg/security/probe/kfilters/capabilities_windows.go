// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kfilters

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

func init() {
	allCapabilities["create"] = Capabilities{
		"create.file.name": {
			PolicyFlags:     PolicyFlagBasename,
			FieldValueTypes: eval.ScalarValueType | eval.PatternValueType,
		},
	}
	allCapabilities["rename"] = Capabilities{
		"rename.file.name": {
			PolicyFlags:     PolicyFlagBasename,
			FieldValueTypes: eval.ScalarValueType | eval.PatternValueType,
		},
	}
	allCapabilities["delete"] = Capabilities{
		"delete.file.name": {
			PolicyFlags:     PolicyFlagBasename,
			FieldValueTypes: eval.ScalarValueType | eval.PatternValueType,
		},
	}
	allCapabilities["write"] = Capabilities{
		"write.file.name": {
			PolicyFlags:     PolicyFlagBasename,
			FieldValueTypes: eval.ScalarValueType | eval.PatternValueType,
		},
	}
}
