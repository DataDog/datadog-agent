// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package processcheck implements a component to handle Process data collection in the Process Agent.
package processcheck

import (
	"github.com/DataDog/datadog-agent/comp/process/types"
)

// team: processes

//nolint:revive // TODO(PROC) Fix revive linter
type Component interface {
	types.CheckComponent
}
