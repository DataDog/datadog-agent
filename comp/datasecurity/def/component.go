// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package datasecurity provides the data security agent component.
//
// The component subscribes to the DEBUG remote-config product on startup. Each
// payload carries scanning rules and a per-integration scan query. For each
// payload the component looks up the related postgres instance credentials,
// builds an in-memory runtime configuration (rules, query, host, password, …)
// and schedules the datasecurity shared-library check once via autodiscovery
// with min_collection_interval: 0. Scanning, querying and event submission are handled by
// that check.
package datasecurity

// team: sensitive-data-scanner

// Component is the data security component interface.
//
// It currently exposes no methods because the component is purely
// side-effectful: it subscribes to RC on startup and logs received configs.
// Methods will be added as functionality grows.
type Component interface{}
