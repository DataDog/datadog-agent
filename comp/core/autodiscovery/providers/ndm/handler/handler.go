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
// Implementations are passed to ndm.NewProvider and must also implement
// Renderer or Snapshotter.
type Handler interface {
	// Key is the document key this handler owns, for example "snmp".
	Key() string
}

// Renderer turns a document key into check configs for autodiscovery.
type Renderer interface {
	Handler

	// Render turns raw, the value of Key() in path's document, into the check
	// configs to schedule for that path and key. It must be free of side
	// effects, and must not block: the component holds its state lock.
	Render(path string, raw json.RawMessage) ([]integration.Config, error)
}

// Snapshotter applies a document key to a component that owns its own
// scheduling, such as discovery.
type Snapshotter interface {
	Handler

	// Snapshot receives, once per update, the value of Key() in every
	// document that carries it, indexed by config path. A path absent from
	// docs no longer carries the key and whatever it asked for is stopped. It
	// returns the paths it could not apply, one error each.
	Snapshot(docs map[string]json.RawMessage) map[string]error
}
