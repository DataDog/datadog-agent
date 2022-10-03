// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package actor

import (
	"context"
	"fmt"
	"testing"
)

func TestWithoutComponents(t *testing.T) {
	actor := Actor{}
	ch := make(chan int)

	run := func(ctx context.Context, alive <-chan struct{}) {
		for {
			select {
			case <-alive:
			case v := <-ch:
				fmt.Printf("GOT: %d\n", v)
			case <-ctx.Done():
				fmt.Println("Stopping")
				return
			}
		}
	}

	actor.Start(run)
	ch <- 1
	ch <- 2
	actor.Stop(context.Background())

	// Output:
	// GOT: 1
	// GOT: 2
	// Stopping
}
