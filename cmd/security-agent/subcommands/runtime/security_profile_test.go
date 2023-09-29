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

func TestShowSecurityProfileCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"runtime", "security-profile", "show"},
		showSecurityProfile,
		func() {})
}

func TestListSecurityProfilesCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"runtime", "security-profile", "list"},
		listSecurityProfiles,
		func() {})
}

func TestSaveSecurityProfileCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"runtime", "security-profile", "save", "--name", "name", "--tag", "tag"},
		saveSecurityProfile,
		func() {})
}
