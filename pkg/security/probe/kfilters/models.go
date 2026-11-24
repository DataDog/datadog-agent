// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kfilters

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

// models holds the approver models of the event types the kernel approves
var models = make(map[eval.EventType]rules.ApproverModel)

// Models returns the approver models of the event types the kernel approves
func Models() map[eval.EventType]rules.ApproverModel {
	return models
}
