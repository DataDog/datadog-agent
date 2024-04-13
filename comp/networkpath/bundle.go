// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package networkpath implements the "networkpath" bundle,
package networkpath

import (
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	"github.com/DataDog/datadog-agent/comp/networkpath/scheduler/schedulerimpl"
)

// team: network-device-monitoring, network-performance-monitoring

// Bundle defines the fx options for this bundle.
var Bundle = fxutil.Bundle(
	schedulerimpl.Module(),
)
