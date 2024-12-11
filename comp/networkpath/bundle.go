// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package networkpath implements the "networkpath" bundle,
package networkpath

import (
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector/npcollectorimpl"
)

// team: Networks network-device-monitoring

// Bundle defines the fx options for this bundle.
func Bundle() fxutil.BundleOptions {
	return fxutil.Bundle(
		npcollectorimpl.Module(),
	)
}
