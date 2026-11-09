// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && kubeapiserver

// Package status implements 'cluster-agent status'.
package status

import (
	"testing"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/command"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestStatusCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"status"},
		run,
		func(cliParams *cliParams) {
			if len(cliParams.args) != 0 {
				t.Fatalf("expected no status section, got %v", cliParams.args)
			}
			if cliParams.list {
				t.Fatal("expected list flag to be false")
			}
		})
}

func TestStatusSectionCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"status", "admission controller"},
		run,
		func(cliParams *cliParams) {
			if len(cliParams.args) != 1 || cliParams.args[0] != "admission controller" {
				t.Fatalf("unexpected status section arguments: %v", cliParams.args)
			}
		})
}

func TestStatusListCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"status", "-l"},
		run,
		func(cliParams *cliParams) {
			if !cliParams.list {
				t.Fatal("expected list flag to be true")
			}
		})
}
