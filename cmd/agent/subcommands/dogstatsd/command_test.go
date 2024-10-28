// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dogstatsd

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"dogstatsd", "top"},
		topContexts,
		func(f *topFlags) {
			assert.Equal(t, "", f.path)
		})
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"dogstatsd", "top", "-p", "foo", "-m", "1", "-t", "2"},
		topContexts,
		func(f *topFlags) {
			assert.Equal(t, "foo", f.path)
			assert.Equal(t, 1, f.nmetrics)
			assert.Equal(t, 2, f.ntags)
		})
}
