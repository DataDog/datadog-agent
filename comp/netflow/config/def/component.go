// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package config exposes the netflow configuration as a component.
package config

// team: ndm-integrations

// Component is the component type.
type Component interface {
	Get() *NetflowConfig
}
