// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fx provides the fx module for the doctor component
package fx

import (
	"github.com/DataDog/datadog-agent/comp/doctor/doctorimpl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: agent-runtimes

// Module defines the fx options for the doctor component
func Module() fxutil.Module {
	return fxutil.Component(
		doctorimpl.Module(),
	)
}
