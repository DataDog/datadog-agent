// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package probe holds probe related files
package probe

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// NewModel returns a new model with some extra field validation
func NewModel(p *Probe[T]) *model.Model {
	return &model.Model{}
}
