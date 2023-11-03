// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package processeventscheck implements a component to handle Process Events data collection in the Process Agent.
package processeventscheck

import (
	"github.com/DataDog/datadog-agent/comp/process/types"
)

// team: processes

type Component interface {
	types.CheckComponent
}
