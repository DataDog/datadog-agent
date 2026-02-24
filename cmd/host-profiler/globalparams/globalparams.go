// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package globalparams contains the global CLI parameters for the host profiler.
package globalparams

import "time"

// GlobalParams contains the values of host profiler global Cobra flags.
//
// A pointer to this type is passed to SubcommandFactory's, but its contents
// are not valid until Cobra calls the subcommand's Run or RunE function.
type GlobalParams struct {
	// ConfFilePath holds the path to the host profiler configuration file.
	ConfFilePath string

	// CoreConfPath holds the path to the Datadog Agent config file.
	CoreConfPath      string
	SyncOnInitTimeout time.Duration
	SyncTimeout       time.Duration
}
