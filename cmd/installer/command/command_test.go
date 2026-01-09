// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package command

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/commands"
)

func TestMakeCommandHasHumanReadableAnnotationOnWindows(t *testing.T) {
	cmd := MakeCommand(nil)

	if runtime.GOOS == "windows" {
		// We expect the default command to redirect to the setup command, which
		// should print human-readable errors
		assert.Equal(t, "true", cmd.Annotations[commands.AnnotationHumanReadableErrors],
			"root command should have human-readable-errors annotation on Windows")
	} else {
		assert.Empty(t, cmd.Annotations[commands.AnnotationHumanReadableErrors],
			"root command should not have human-readable-errors annotation on non-Windows platforms")
	}
}
