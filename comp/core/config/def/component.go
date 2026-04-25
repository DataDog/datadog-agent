// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package config provides the config component interface for the Datadog Agent.
package config

import (
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// team: agent-configuration

// Component is the component type.
type Component interface {
	pkgconfigmodel.ReaderWriter

	// Warnings returns config warnings collected during setup.
	Warnings() *pkgconfigmodel.Warnings
}
