// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package locallogtailer provides the locallogtailer component bundle.
package locallogtailer

import (
	locallogtailerfx "github.com/DataDog/datadog-agent/comp/logs/locallogtailer/fx"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: q-branch

// Bundle defines the fx options for the localreader bundle.
func Bundle() fxutil.BundleOptions {
	return fxutil.Bundle(
		locallogtailerfx.Module(),
	)
}
