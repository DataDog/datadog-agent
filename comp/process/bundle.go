// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package process implements the "process" bundle, providing components for the Process Agent
//
// The constituent components serve as utilities and are mostly independent of
// one another.  Other components should depend on any components they need.
//
// This bundle does not depend on any other bundles.
package process

import (
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/process/connectionscheck"
	"github.com/DataDog/datadog-agent/comp/process/containercheck"
	"github.com/DataDog/datadog-agent/comp/process/hostinfo"
	"github.com/DataDog/datadog-agent/comp/process/podcheck"
	"github.com/DataDog/datadog-agent/comp/process/processcheck"
	"github.com/DataDog/datadog-agent/comp/process/processdiscoverycheck"
	"github.com/DataDog/datadog-agent/comp/process/processeventscheck"
	"github.com/DataDog/datadog-agent/comp/process/profiler"
	"github.com/DataDog/datadog-agent/comp/process/rtcontainercheck"
	"github.com/DataDog/datadog-agent/comp/process/runner"
	"github.com/DataDog/datadog-agent/comp/process/submitter"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: processes

// Bundle defines the fx options for this bundle.
var Bundle = fxutil.Bundle(
	runner.Module,
	submitter.Module,
	profiler.Module,

	// Checks
	connectionscheck.Module,
	containercheck.Module,
	podcheck.Module,
	processcheck.Module,
	processeventscheck.Module,
	rtcontainercheck.Module,
	processdiscoverycheck.Module,

	hostinfo.Module,
	core.Bundle,
)
