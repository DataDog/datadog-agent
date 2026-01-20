// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !observer

// Package fx defines the fx options for the observer component.
package fx

import (
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for the observer component.
// This is a noop implementation when the observer build tag is not set.
func Module() fxutil.Module {
	return fxutil.Component()
}
