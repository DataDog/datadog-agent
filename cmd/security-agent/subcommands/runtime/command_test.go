// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package runtime

import (
	"testing"

	"github.com/DataDog/datadog-agent/cmd/security-agent/command"
	"github.com/DataDog/datadog-agent/cmd/system-probe/subcommands/runtime"
	"github.com/DataDog/datadog-agent/cmd/system-probe/subcommands/runtime/policy"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestDumpProcessCacheCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"runtime", "process-cache", "dump"},
		runtime.DumpProcessCache,
		func() {})
}

func TestDumpNetworkNamespaceCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"runtime", "network-namespace", "dump"},
		runtime.DumpNetworkNamespace,
		func() {})
}

func TestDumpDiscardersCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"runtime", "discarders", "dump"},
		runtime.DumpDiscarders,
		func() {})
}

func TestEvalCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"runtime", "policy", "eval", "--rule-id=10", "--event-file=file"},
		policy.EvaluateRule,
		func() {})
}

func TestCheckPoliciesCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"runtime", "policy", "check"},
		policy.CheckPolicies,
		func() {})
}

func TestReloadRuntimePoliciesCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"runtime", "policy", "reload"},
		policy.ReloadRuntimePolicies,
		func() {})
}

func TestRunRuntimeSelfTestCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"runtime", "self-test"},
		runtime.RunRuntimeSelfTest,
		func() {})
}
