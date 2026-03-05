// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package logondurationimpl implements the logon duration component
package logondurationimpl

import (
	logonduration "github.com/DataDog/datadog-agent/comp/logonduration/def"
)

// Provides defines what this component provides
type Provides struct {
	Comp logonduration.Component
}
