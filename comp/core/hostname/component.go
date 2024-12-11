// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package hostname exposes hostname.Get() as a component.
package hostname

import "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"

// team: agent-shared-components

// Component is the component type.
type Component = hostnameinterface.Component
