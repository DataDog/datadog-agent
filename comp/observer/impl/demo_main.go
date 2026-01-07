// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"context"
	"fmt"
	"time"
)

// RunDemo runs the full demo scenario and blocks until complete.
// timeScale controls speed: 1.0 = realtime (40s), 0.1 = 10x faster (4s)
func RunDemo(timeScale float64) {
	if timeScale <= 0 {
		timeScale = 0.1
	}

	fmt.Printf("Starting observer demo (timeScale=%.2f, duration=%.1fs)\n", timeScale, 40.0*timeScale)
	fmt.Println("---")

	// Create observer using the standard constructor
	provides := NewComponent(Requires{})
	observer := provides.Comp

	// Get a handle for the demo generator
	handle := observer.GetHandle("demo")

	// Create and configure the data generator
	generator := NewDataGenerator(handle, GeneratorConfig{
		TimeScale:     timeScale,
		BaselineNoise: 0.1,
	})

	// Run the generator with a timeout for the scenario duration (40s scaled)
	scenarioDuration := time.Duration(float64(40*time.Second) * timeScale)
	ctx, cancel := context.WithTimeout(context.Background(), scenarioDuration)
	defer cancel()

	generator.Run(ctx)

	// Small buffer to let final events flush through the pipeline
	time.Sleep(time.Duration(float64(500*time.Millisecond) * timeScale))

	fmt.Println("---")
	fmt.Println("Demo complete.")
}
