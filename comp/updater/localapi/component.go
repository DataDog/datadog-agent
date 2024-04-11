// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package localapi is the updater local api component.
package localapi

import (
	"github.com/DataDog/datadog-agent/pkg/updater"
)

// team: fleet

// Component is the interface for the updater local api component.
type Component interface {
	updater.LocalAPI
}
