// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

package testutil

import "github.com/DataDog/datadog-agent/pkg/network/config"

// Config returns a network.Config setup for test purposes
func Config() *config.Config {
	return config.New()
}
