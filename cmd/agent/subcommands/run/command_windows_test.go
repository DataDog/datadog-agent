// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package run

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestStartAgentWithDefaults(t *testing.T) {
	fxutil.TestOneShot(t,
		func() {
			ctxChan := make(<-chan context.Context)
			_, err := StartAgentWithDefaults(ctxChan, command.ProductComposition{})
			require.NoError(t, err)
		})
}
