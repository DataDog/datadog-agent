// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package main

import (
	"context"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/stretchr/testify/require"
)

func TestFxRun(t *testing.T) {
	svc := service{}
	ctx := context.Background()
	fxutil.TestOneShot(t, func() {
		err := svc.Run(ctx)
		require.NoError(t, err)
	})
}
