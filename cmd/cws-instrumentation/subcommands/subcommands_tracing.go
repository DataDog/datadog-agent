// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && !cws_instrumentation_injector_only

// Package subcommands is used to list the subcommands of CWS injector
package subcommands

import "github.com/DataDog/datadog-agent/cmd/cws-instrumentation/subcommands/tracecmd"

func init() {
	CWSInjectorSubcommands = append(CWSInjectorSubcommands, tracecmd.Command)
}
