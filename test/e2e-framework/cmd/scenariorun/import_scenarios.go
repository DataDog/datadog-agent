// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package main

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenario/scenarios/agenthealth"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenario/scenarios/ec2host"
)

// registerScenarios registers all built-in scenarios. Add new scenarios here.
func registerScenarios() {
	ec2host.Register()
	agenthealth.Register()
}
