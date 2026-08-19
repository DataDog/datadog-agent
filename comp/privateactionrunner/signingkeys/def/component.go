// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package signingkeys defines the Core Agent's PAR signing-key snapshot component.
package signingkeys

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/signingkeys"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

// Snapshot is the latest authoritative AP_RUNNER_KEYS state.
type Snapshot struct {
	Keys         []signingkeys.Key
	Revision     uint64
	UpdatedAt    time.Time
	Initialized  bool
	ConfigStatus pbgo.ConfigStatus
	Unchanged    bool
}

// team: action-platform

// Component stores and serves the Core Agent's AP_RUNNER_KEYS snapshot.
type Component interface {
	Get(knownRevision uint64) (Snapshot, error)
}
