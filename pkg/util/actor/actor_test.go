// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package actor

import (
	"context"
	"fmt"
	"testing"

	"go.uber.org/fx"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func Test(t *testing.T) {
	input := make(chan int)
	output := make(chan string, 2)
	fxutil.Test(t, fx.Options(
		fx.Invoke(func(lc fx.Lifecycle) {
			run := func(ctx context.Context, alive <-chan struct{}) {
				for {
					select {
					case <-alive:
					case v := <-input:
						output <- fmt.Sprintf("GOT %d", v)
					case <-ctx.Done():
						close(output)
						return
					}
				}
			}
			New(lc, run)
		}),
	), func() {
		input <- 1
		input <- 2
	})

	require.Equal(t, <-output, "GOT 1")
	require.Equal(t, <-output, "GOT 2")
	require.Equal(t, <-output, "")
}
