// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package handler defines the contract between the NDM Remote Configuration
// component and the features that own a key of its document.
package handler

import (
	"encoding/json"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
)

// Handler owns one top-level key of the NDM Remote Configuration document.
// Implementations are passed to ndm.NewProvider.
type Handler interface {
	// Key is the document key this handler owns, for example "snmp".
	Key() string

	// Render turns raw, the value of Key() in path's document, into the check
	// configs to schedule for that path and key. It must be free of side
	// effects, and must not block: the component holds its state lock.
	Render(path string, raw json.RawMessage) ([]integration.Config, error)
}
