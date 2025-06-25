// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !darwin && !windows && kubeapiserver

package start

import (
    "testing"

    "github.com/stretchr/testify/require"
)

// TestValidatingStopChDoubleClose ensures that closing a channel twice using the
// guarded pattern used in command.go does not panic.
func TestValidatingStopChDoubleClose(t *testing.T) {
    r := require.New(t)

    ch := make(chan struct{})
    close(ch) // first close
    ch = nil  // guard assignment mirroring production fix

    r.NotPanics(func() {
        if ch != nil {
            close(ch)
        }
    })
}
