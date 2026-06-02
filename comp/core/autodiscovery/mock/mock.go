// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package mock provides the mock for the autodiscovery component.
package mock

import autodiscovery "github.com/DataDog/datadog-agent/comp/core/autodiscovery/def"

// Mock implements mock-specific methods.
type Mock interface {
	autodiscovery.Component
}
