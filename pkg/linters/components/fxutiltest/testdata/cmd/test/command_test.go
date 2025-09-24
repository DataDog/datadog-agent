// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package test

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestCommand(t *testing.T) {
	// Only test "top" command - "dump-contexts" is missing a test
	fxutil.TestOneShotSubcommand(t,
		Commands(),
		[]string{"top"},
		myFunction,
		func() {
		})
}
