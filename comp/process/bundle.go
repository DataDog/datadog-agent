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
	"github.com/DataDog/datadog-agent/comp/process/agent/agentimpl"
	"github.com/DataDog/datadog-agent/comp/process/apiserver"
	"github.com/DataDog/datadog-agent/comp/process/connectionscheck/connectionscheckimpl"
	"github.com/DataDog/datadog-agent/comp/process/containercheck/containercheckimpl"
	"github.com/DataDog/datadog-agent/comp/process/expvars/expvarsimpl"
	"github.com/DataDog/datadog-agent/comp/process/forwarders/forwardersimpl"
	"github.com/DataDog/datadog-agent/comp/process/hostinfo/hostinfoimpl"
	"github.com/DataDog/datadog-agent/comp/process/podcheck/podcheckimpl"
	"github.com/DataDog/datadog-agent/comp/process/processcheck/processcheckimpl"
	"github.com/DataDog/datadog-agent/comp/process/processdiscoverycheck/processdiscoverycheckimpl"
	"github.com/DataDog/datadog-agent/comp/process/processeventscheck/processeventscheckimpl"
	"github.com/DataDog/datadog-agent/comp/process/profiler/profilerimpl"
	"github.com/DataDog/datadog-agent/comp/process/rtcontainercheck/rtcontainercheckimpl"
	"github.com/DataDog/datadog-agent/comp/process/runner/runnerimpl"
	"github.com/DataDog/datadog-agent/comp/process/submitter/submitterimpl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: processes

// Bundle defines the fx options for this bundle.
func Bundle() fxutil.BundleOptions {
	return fxutil.Bundle(
		runnerimpl.Module(),
		submitterimpl.Module(),
		profilerimpl.Module(),

		// Checks
		connectionscheckimpl.Module(),
		containercheckimpl.Module(),
		podcheckimpl.Module(),
		processcheckimpl.Module(),
		processeventscheckimpl.Module(),
		rtcontainercheckimpl.Module(),
		processdiscoverycheckimpl.Module(),

		agentimpl.Module(),

		hostinfoimpl.Module(),
		expvarsimpl.Module(),
		apiserver.Module(),
		forwardersimpl.Module(),
	)
}
