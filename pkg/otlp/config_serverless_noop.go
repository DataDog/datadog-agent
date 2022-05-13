// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build serverless
// +build serverless

package otlp

import (
	"github.com/DataDog/datadog-agent/pkg/config"
)

// IsEnabled always returns false since OTLP is disabled in the serverless agent flavor
func IsEnabled(cfg config.Config) bool {
	return false
}

// IsDisplayed checks if the OTLP section should be rendered in the Agent
func IsDisplayed() bool {
	return false
}
