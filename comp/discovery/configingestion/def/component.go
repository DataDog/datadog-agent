// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package configingestion provides experimental config file discovery and ingestion.
package configingestion

// team: agent-discovery

// Component is the config file ingestion component interface.
// It has no exported methods; lifecycle is managed via Fx hooks.
type Component interface{}
