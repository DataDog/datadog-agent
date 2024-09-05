// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package config

import (
	"github.com/DataDog/datadog-agent/pkg/config/env"
)

var (
	// SetFeatures is alias from env
	SetFeatures = env.SetFeatures
	// SetFeaturesNoCleanup is alias from env
	SetFeaturesNoCleanup = env.SetFeaturesNoCleanup
)
