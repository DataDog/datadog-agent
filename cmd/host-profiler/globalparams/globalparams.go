// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package globalparams contains the global CLI parameters for the host profiler.
package globalparams

// GlobalParams contains the values of host profiler global Cobra flags.
//
// A pointer to this type is passed to SubcommandFactory's, but its contents
// are not valid until Cobra calls the subcommand's Run or RunE function.
type GlobalParams struct {
	// ConfFilePath holds the path to the host profiler configuration file.
	ConfFilePath string

	// CoreConfPath holds the path to the Datadog Agent config file.
	CoreConfPath string
}

// ConfigURI returns the appropriate configuration URI based on the operational mode.
// In bundled mode (CoreConfPath set), it returns "dd:" to use the agentprovider,
// which generates OTEL config from the Agent configuration.
// In standalone mode (ConfFilePath set), it returns the file path to the OTEL config.
func (g *GlobalParams) ConfigURI() string {
	if g.CoreConfPath != "" {
		return "dd:"
	}
	return g.ConfFilePath
}
