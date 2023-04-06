// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package forwarder implements the "forwarder" bundle
package forwarder

import (
	"github.com/DataDog/datadog-agent/comp/forwarder/forwarder"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: Agent Shared components

// Bundle defines the fx options for this bundle.
var Bundle = fxutil.Bundle(
	forwarder.Module,
)
