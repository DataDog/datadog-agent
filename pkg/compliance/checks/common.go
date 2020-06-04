// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package checks

import (
	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func evalCondition(property string, condition *compliance.Condition) bool {
	switch condition.Operation {
	case compliance.OpExists, "":
		return property != ""

	case compliance.OpEqual:
		return property == condition.Value
	default:
		log.Warnf("Unsupported operation in condition: %s", condition.Operation)
		return false
	}
}