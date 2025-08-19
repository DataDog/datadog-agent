// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package remoteconfig defines the fx options for the Bundle
package remoteconfig

import (
	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient/rcclientimpl"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcstatus/rcstatusimpl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: remote-config

// Bundle defines the fx options for this bundle.
func Bundle() fxutil.BundleOptions {
	return fxutil.Bundle(
		rcclientimpl.Module(),
		rcstatusimpl.Module(),
	)
}
