// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package runtime

import (
	"testing"

	"github.com/DataDog/datadog-agent/cmd/security-agent/command"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestListActivityDumpsCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"runtime", "activity-dump", "list"},
		listActivityDumps,
		func() {})
}

func TestStopActivityDumpCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"runtime", "activity-dump", "stop"},
		stopActivityDump,
		func() {})
}

func TestGenerateActivityDumpCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"runtime", "activity-dump", "generate", "dump"},
		generateActivityDump,
		func() {})
}

func TestGenerateEncodingFromActivityDumpCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"runtime", "activity-dump", "generate", "encoding", "--input", "file"},
		generateEncodingFromActivityDump,
		func() {})
}

func TestDiffActivityDumpCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"runtime", "activity-dump", "diff"},
		diffActivityDump,
		func() {})
}
