package events

import (
	"sync"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
)

// TestConcurrentMapWritesReproduction attempts to reproduce the concurrent map writes issue
// by simulating the worker pool behavior with multiple goroutines executing callbacks concurrently
func TestConcurrentMapWritesReproduction(t *testing.T) {
	const numWorkers = 50
	const numOperations = 1000

	// Create a metric group that will be accessed concurrently
	metricGroup := telemetry.NewMetricGroup("test.concurrent")

	// This simulates what happens in the worker pool - multiple goroutines
	// executing callback functions that access telemetry metrics
	var wg sync.WaitGroup
	wg.Add(numWorkers)

	// Channel to simulate the worker pool jobs
	jobs := make(chan func(), numOperations)

	// Start workers (similar to the worker pool goroutines)
	for i := 0; i < numWorkers; i++ {
		go func(workerID int) {
			defer wg.Done()
			for job := range jobs {
				job()
			}
		}(i)
	}

	// Create jobs that access telemetry metrics concurrently
	for i := 0; i < numOperations; i++ {
		operation := i
		jobs <- func() {
			// This simulates what protocol handlers do - create/access telemetry counters
			counter := metricGroup.NewCounter("operations", "type:test")
			counter.Add(1)

			// Also test gauge creation/access
			gauge := metricGroup.NewGauge("status", "worker:test")
			gauge.Set(int64(operation))
		}
	}

	close(jobs)
	wg.Wait()
}

// TestConcurrentMetricCreation specifically tests concurrent metric creation
// which is the most likely source of the race condition
func TestConcurrentMetricCreation(t *testing.T) {
	const numGoroutines = 100
	const numMetrics = 50

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// All goroutines will try to create metrics with the same names concurrently
	for i := 0; i < numGoroutines; i++ {
		go func(routineID int) {
			defer wg.Done()

			metricGroup := telemetry.NewMetricGroup("test.concurrent.creation")

			for j := 0; j < numMetrics; j++ {
				// Try to create metrics with the same names from multiple goroutines
				// This should trigger the race condition in the registry
				counter := metricGroup.NewCounter("test_counter", "routine:test")
				counter.Add(1)

				gauge := metricGroup.NewGauge("test_gauge", "routine:test")
				gauge.Set(int64(routineID))
			}
		}(i)
	}

	wg.Wait()
}

// TestWorkerPoolRaceCondition simulates the exact worker pool scenario from the stack trace
func TestWorkerPoolRaceCondition(t *testing.T) {
	// Create a worker pool similar to the one in the codebase
	pool, err := newWorkerPool(32)
	if err != nil {
		t.Fatalf("Failed to create worker pool: %v", err)
	}
	defer pool.Stop()

	// Create telemetry that will be accessed by worker goroutines
	metricGroup := telemetry.NewMetricGroup("test.worker.pool")

	const numJobs = 1000
	var wg sync.WaitGroup
	wg.Add(numJobs)

	// Submit jobs to the worker pool that access telemetry metrics
	for i := 0; i < numJobs; i++ {
		jobID := i
		pool.Do(func() {
			defer wg.Done()

			// This simulates what protocol handlers do in their callback functions
			counter := metricGroup.NewCounter("events", "job:test")
			counter.Add(1)

			// Simulate some processing time
			time.Sleep(time.Microsecond)

			gauge := metricGroup.NewGauge("processed", "job:test")
			gauge.Set(int64(jobID))
		})
	}

	wg.Wait()
}
