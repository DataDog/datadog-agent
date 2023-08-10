// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package tailers

import (
	"github.com/DataDog/datadog-agent/pkg/logs/internal/status"
)

// Tailer is the base interface for a tailer.
type Tailer interface {
	GetId() string
	GetType() string
	GetInfo() *status.InfoRegistry
}
