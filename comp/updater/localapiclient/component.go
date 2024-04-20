// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package localapiclient provides the local API client component.
package localapiclient

import "github.com/DataDog/datadog-agent/pkg/installer"

// team: fleet

// Component is the component type.
type Component interface {
	installer.LocalAPIClient
}
