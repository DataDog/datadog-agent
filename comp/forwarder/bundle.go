// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package forwarder implements the "forwarder" bundle
package forwarder

import (
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: agent-shared-components

// Bundle defines the fx options for this bundle.
func Bundle() fxutil.BundleOptions {
	return fxutil.Bundle(
		defaultforwarder.Module())
}
