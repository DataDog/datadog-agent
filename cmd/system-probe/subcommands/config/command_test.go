// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package config implements 'system-probe config'.
package config

import (
	"testing"

	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestShowRuntimeConfigurationCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"config"},
		showRuntimeConfiguration,
		func() {})
}

func TestListRuntimeConfigurableValueCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"config", "list-runtime"},
		listRuntimeConfigurableValue,
		func() {})
}

func TestSetConfigValueCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"config", "set", "setting", "value"},
		setConfigValue,
		func() {})
}

func TestGetConfigValueCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"config", "get", "setting"},
		getConfigValue,
		func() {})
}
