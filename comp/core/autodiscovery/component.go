// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package autodiscovery provides the autodiscovery component for the Datadog Agent
package autodiscovery

import (
	addef "github.com/DataDog/datadog-agent/comp/core/autodiscovery/def"
)

// Component is the component type.
// Deprecated: use comp/core/autodiscovery/def instead.
type Component = addef.Component
