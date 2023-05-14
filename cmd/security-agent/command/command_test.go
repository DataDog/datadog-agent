// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package command

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/cmd/security-agent/flags"
)

func TestMakeCommand(t *testing.T) {
	var subcommandFactories []SubcommandFactory
	cmd := MakeCommand(subcommandFactories)

	// The pflags package stringifies string arrays as CSVs, then adds back the square brackets
	require.Equal(t, strings.ReplaceAll(fmt.Sprint(defaultSecurityAgentConfigFilePaths), " ", ","), cmd.Flag(flags.CfgPath).Value.String(), "cfgpath values not matching")

	//TODO: add test to ensure setting of no-color
}
