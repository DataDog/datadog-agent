// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package tailers provides the base interface and tracking for log tailers
package tailers

import (
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
)

// Tailer is the base interface for a tailer.
type Tailer interface {
	GetID() string
	GetType() string
	GetInfo() *status.InfoRegistry
}
