package main

import (
	"fmt"
	"runtime"

	"github.com/DataDog/datadog-agent/pkg/config/setup"
)

// go build -o standalone-benchmark/main github.com/DataDog/datadog-agent/standalone-benchmark

func main() {
	// Get the global config, which requires it to have been constructed
	// and setup by pkg/config/setup/InitConfig()
	_ = setup.Datadog()

	// Collect garbage, in order to measure what was allocated and still in use
	runtime.GC()
	mstats := runtime.MemStats{}
	runtime.ReadMemStats(&mstats)
	fmt.Printf("allocated memory: %d\n", mstats.Alloc)
}
