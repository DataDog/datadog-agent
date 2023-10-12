// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build !otlp

package otlp

import (
	"github.com/DataDog/datadog-agent/pkg/config"
)

// IsEnabled checks if OTLP pipeline is enabled in a given config.
func IsEnabled(cfg config.Config) bool {
	return false
}

// IsDisplayed checks if the OTLP section should be rendered in the Agent
func IsDisplayed() bool {
	return false
}

// Pipeline is an OTLP pipeline.
type Pipeline struct{}

// Stop the OTLP pipeline.
func (p *Pipeline) Stop() {}
