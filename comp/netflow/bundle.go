// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package netflow implements the "netflow" bundle, which listens for netflow
// packets, processes them, and forwards relevant data to the backend.
package netflow

import (
	"github.com/DataDog/datadog-agent/comp/netflow/config"
	"github.com/DataDog/datadog-agent/comp/netflow/server"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: ndm-integrations

// Bundle defines the fx options for this bundle.
func Bundle() fxutil.BundleOptions {
	return fxutil.Bundle(
		server.Module(),
		config.Module())
}
