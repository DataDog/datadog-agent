// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package containercheck implements a component to handle Container data collection in the Process Agent.
package containercheck

import (
	"github.com/DataDog/datadog-agent/comp/process/types"
)

// team: container-experiences

// Component is the component type.
type Component interface {
	types.CheckComponent
}
