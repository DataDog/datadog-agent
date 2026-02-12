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
	// StandaloneConfigPath holds the path to the standalone host profiler configuration file.
	StandaloneConfigPath string

	// BundledConfigPath holds the path to the Datadog Agent config file for bundled mode.
	BundledConfigPath string
}

// ConfigURI returns the appropriate configuration URI based on the mode.
// In bundled mode (BundledConfigPath set), it returns "dd:" to use the agentprovider.
// In standalone mode (StandaloneConfigPath set), it returns the file path.
func (g *GlobalParams) ConfigURI() string {
	if g.BundledConfigPath != "" {
		return "dd:"
	}
	return g.StandaloneConfigPath
}
