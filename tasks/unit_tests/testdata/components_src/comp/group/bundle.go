// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package group bundles together components
package group

import "github.com/DataDog/datadog-agent/pkg/util/fxutil"

// team: agent-shared-components

func Bundle() fxutil.BundleOptions {
	return fxutil.Bundle()
}
