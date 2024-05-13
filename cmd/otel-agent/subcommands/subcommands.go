// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package subcommands contains the subcommands of the otel-agent.
package subcommands

import "strings"

// GlobalParams contains the values of agent-global Cobra flags.
//
// A pointer to this type is passed to SubcommandFactory's, but its contents
// are not valid until Cobra calls the subcommand's Run or RunE function.
type GlobalParams struct {
	ConfPaths  []string
	Sets       []string
	ConfigName string
	LoggerName string
}

// Set is called by Cobra when a flag is set.
func (s *GlobalParams) Set(val string) error {
	s.ConfPaths = append(s.ConfPaths, val)
	return nil
}

// String returns a string representation of the GlobalParams.
func (s *GlobalParams) String() string {
	return "[" + strings.Join(s.ConfPaths, ", ") + "]"
}
