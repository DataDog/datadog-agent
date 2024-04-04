// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package agent implements the "agent" bundle,
package agent

import (
	"github.com/DataDog/datadog-agent/comp/agent/expvarserver/expvarserverimpl"
	"github.com/DataDog/datadog-agent/comp/agent/jmxlogger/jmxloggerimpl"
	"github.com/DataDog/datadog-agent/comp/agent/metadatascheduler/metadataschedulerimpl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: agent-shared-components

// Bundle defines the fx options for this bundle.
func Bundle() fxutil.BundleOptions {
	return fxutil.Bundle(
		metadataschedulerimpl.Module(),
		jmxloggerimpl.Module(),
		expvarserverimpl.Module(),
	)
}
