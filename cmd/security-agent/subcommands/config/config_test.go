// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package config holds config related files
package config

import (
	"testing"

	"github.com/DataDog/datadog-agent/cmd/security-agent/command"
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
		[]string{"config", "get", "setting", "value"},
		getConfigValue,
		func() {})
}

func TestShowConfigBySource(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"config", "by-source"},
		showRuntimeConfigurationBySource,
		func() {})
}
