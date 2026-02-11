// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package subcommands contains the subcommands of the mini-agent.
package subcommands

// GlobalParams contains the values of agent-global Cobra flags.
type GlobalParams struct {
	ConfigName string
	LoggerName string
	ConfPath   string
}
