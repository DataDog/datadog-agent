// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package mock

import (
	"github.com/DataDog/datadog-agent/pkg/logs/status"
)

// NewTracker returns a new mock Tracker
func NewTracker() *status.Tracker {
	return status.NewTracker("")
}
