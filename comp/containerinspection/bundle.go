// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package containerinspection implements kubernetes container inspection.
package containerinspection

// team: apm-ecosystems

import (
	"github.com/DataDog/datadog-agent/comp/containerinspection/client"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Bundle defines the fx options for this bundle.
func Bundle() fxutil.BundleOptions {
	return fxutil.Bundle(client.Module())
}
