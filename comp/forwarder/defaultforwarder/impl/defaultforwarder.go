// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package defaultforwarderimpl implements the defaultforwarder component.
//
// # V2 migration status: WIP
//
// This package is a placeholder for the V2 migration of
// comp/forwarder/defaultforwarder. The implementation currently lives in the
// root package. A follow-up PR will:
//
//  1. Move all implementation files (default_forwarder.go, domain_forwarder.go,
//     forwarder_health.go, noop_forwarder.go, params.go, shared_connection.go,
//     status.go, sync_forwarder.go, telemetry.go, worker.go, etc.) into this
//     package with package name defaultforwarderimpl.
//
//  2. Replace the duplicate type definitions in the root package with type
//     aliases pointing to def/ (e.g. type Response = defpkg.Response).
//
//  3. Update all 55+ callers to import the appropriate sub-package.
//
// Until that follow-up lands, the root package remains the authoritative
// implementation and callers should continue to use it.
package defaultforwarderimpl

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	"github.com/DataDog/datadog-agent/comp/core/status"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	defpkg "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/def"
)

// Requires defines the dependencies for the defaultforwarder component.
//
// TODO(migration): populate this with the proper fields once the implementation
// moves here. The current root-package forwarder.go uses fx.In-tagged structs;
// this will use plain Go fields as per the V2 pattern.
type Requires struct {
	Config    config.Component
	Log       log.Component
	Lifecycle compdef.Lifecycle
	Secrets   secrets.Component
}

// Provides defines the output of the defaultforwarder component.
type Provides struct {
	Comp           defpkg.Component
	StatusProvider status.InformationProvider
}

// NewComponent creates a new defaultforwarder component.
//
// TODO(migration): this is currently a stub. Once the implementation files are
// moved to this package, this constructor will call the local implementation.
// For now, callers should use the root-package Module() function directly.
func NewComponent(_ Requires) (Provides, error) {
	// Implementation pending full V2 migration.
	// See package-level doc comment for the migration plan.
	panic("defaultforwarderimpl.NewComponent: not yet implemented — use the root package Module() instead")
}
