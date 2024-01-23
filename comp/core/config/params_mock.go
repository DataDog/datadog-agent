// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test
// +build test

package config

import (
	pkgconfigenv "github.com/DataDog/datadog-agent/pkg/config/env"
)

// MockParams defines the parameter for the mock config.
// It is designed to be used with `fx.Replace` which replaces the default
// empty value of `MockParams`.
//
//	fx.Replace(configComponent.MockParams{Overrides: overrides})
type MockParams struct {
	Params

	// Overrides is a parameter used to override values of the config
	Overrides map[string]interface{}

	// Features is a parameter to set features for the mock config
	Features []pkgconfigenv.Feature

	// SetupConfig sets up the config as if it weren't a mock; essentially a full init
	SetupConfig bool
}
