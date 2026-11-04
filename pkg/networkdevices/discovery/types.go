// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package discovery holds the shapes the NDM discovery IPC endpoint exchanges
// with the Private Action Runner.
package discovery

import "errors"

// ErrInvalidRequest is wrapped by every error a malformed trigger request produces.
var ErrInvalidRequest = errors.New("invalid discovery request")

// TriggerRequest asks the Agent to sweep one configured range now, rather than
// waiting for its next scheduled run.
type TriggerRequest struct {
	AutodiscoveryID string `json:"autodiscovery_id"`
}
