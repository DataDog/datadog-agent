// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package metadata implements the "metadata" bundle, providing services and support for all the metadata payload sent
// by the Agent.
package metadata

import (
	"github.com/DataDog/datadog-agent/comp/metadata/host"
	"github.com/DataDog/datadog-agent/comp/metadata/resources"
	"github.com/DataDog/datadog-agent/comp/metadata/runner"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: agent-shared-components

// Bundle defines the fx options for this bundle.
var Bundle = fxutil.Bundle(
	runner.Module,
	resources.Module,
	host.Module,
)

// MockBundle defines the mock fx options for this bundle.
var MockBundle = fxutil.Bundle()
