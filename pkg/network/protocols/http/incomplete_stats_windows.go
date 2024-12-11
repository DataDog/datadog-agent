// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm

package http

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/network/config"
)

// Windows does not have incomplete http transactions because flows in the windows driver
// see both directions of traffic
type incompleteBuffer struct{}

// NewIncompleteBuffer returns a new incompleteBuffer instance
func NewIncompleteBuffer(*config.Config, *Telemetry) IncompleteBuffer {
	return &incompleteBuffer{}
}

//nolint:revive // TODO(WKIT) Fix revive linter
func (b *incompleteBuffer) Add(Transaction) {}

//nolint:revive // TODO(WKIT) Fix revive linter
func (b *incompleteBuffer) Flush(time.Time) []Transaction { return nil }
