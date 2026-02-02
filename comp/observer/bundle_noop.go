// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !observer

// Package observer bundles the observer component.
package observer

import (
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Bundle defines the fx options for the observer bundle.
// This is a noop implementation when the observer build tag is not set.
func Bundle() fxutil.BundleOptions {
	return fxutil.Bundle()
}
