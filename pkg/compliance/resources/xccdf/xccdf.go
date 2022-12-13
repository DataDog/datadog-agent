// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package xccdf

import (
	"github.com/DataDog/datadog-agent/pkg/compliance/resources"
)

var reportedFields = []string{}

func init() {
	resources.RegisterHandler("xccdf", resolve, reportedFields)
}
