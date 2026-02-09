// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"time"

	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
	"gopkg.in/DataDog/dd-trace-go.v1/profiler"
)

var (
	serviceName  = flag.String("service", "go-traced-app", "Service name for traces and profiling")
	agentAddr    = flag.String("agent", "localhost:8126", "Datadog agent address")
	env          = flag.String("env", "dev", "Environment name")
	statsdAddr   = flag.String("statsd", "localhost:8125", "DogStatsD address")
	logFilePath  = flag.String("log-file", "/tmp/go-traced-app.log", "Log file path")
	startDelay   = flag.Duration("start-delay", 0, "Delay before starting the simulation (e.g. 5s, 1m)")
	statsdClient ddgostatsd.ClientInterface
)

func main() {
	flag.Parse()

	logFile, err := os.OpenFile(*logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		log.Printf("Warning: Failed to open log file %s: %v", *logFilePath, err)
	} else {
		log.SetOutput(io.MultiWriter(os.Stdout, logFile))
		defer logFile.Close()
	}

	// Start the Datadog tracer
	// Configure to send to local trace-agent (default: localhost:8126)
	tracer.Start(
		tracer.WithAgentAddr(*agentAddr),
		tracer.WithService(*serviceName),
		tracer.WithEnv(*env),
		tracer.WithServiceVersion("1.0.0"),
		tracer.WithRuntimeMetrics(), // Enable runtime metrics (CPU, memory, goroutines)
	)
	defer tracer.Stop()

	// Start the profiler for CPU and memory profiling
	err = profiler.Start(
		profiler.WithService(*serviceName),
		profiler.WithEnv(*env),
		profiler.WithVersion("1.0.0"),
		profiler.WithAgentAddr(*agentAddr),
		profiler.WithProfileTypes(
			profiler.CPUProfile,
			profiler.HeapProfile,
			profiler.GoroutineProfile,
			profiler.MutexProfile,
		),
		profiler.WithPeriod(10*time.Second), // Upload profiles every 10 seconds
	)
	if err != nil {
		log.Printf("Warning: Failed to start profiler: %v", err)
	}
	defer profiler.Stop()

	// Initialize DogStatsD client
	client, err := ddgostatsd.New(*statsdAddr,
		ddgostatsd.WithNamespace("app."),
		ddgostatsd.WithTags([]string{fmt.Sprintf("service:%s", *serviceName), fmt.Sprintf("env:%s", *env)}),
	)
	if err != nil {
		log.Printf("Warning: Failed to create DogStatsD client: %v", err)
	} else {
		statsdClient = client
		defer client.Close()
	}

	log.Printf("Go Traced App started (service: %s)", *serviceName)
	log.Printf("Sending traces to %s", *agentAddr)
	log.Printf("Sending metrics to %s", *statsdAddr)
	log.Printf("Environment: %s", *env)
	log.Println("Press Ctrl+C to stop")

	if *startDelay > 0 {
		log.Printf("Delaying simulation start by %s", startDelay.String())
		time.Sleep(*startDelay)
	}

	// Run the demo workload
	runDemo()
}

func runDemo() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		log.Println("Synthetic error: simulated request failure for log collection")
		// Create a root span for a simulated web request
		span := tracer.StartSpan("web.request",
			tracer.ResourceName("/api/users"),
			tracer.Tag("http.method", "GET"),
			tracer.Tag("http.url", "/api/users"),
		)

		// Create a context with the span embedded
		ctx := tracer.ContextWithSpan(context.Background(), span)

		// Simulate some work
		processRequest(ctx)

		span.Finish()
		log.Println("Sent trace with multiple spans")
	}
}

func processRequest(ctx context.Context) {
	// Create a child span for database operation
	dbSpan, ctx := tracer.StartSpanFromContext(ctx, "db.query",
		tracer.ResourceName("SELECT * FROM users"),
		tracer.Tag("db.type", "postgres"),
		tracer.Tag("db.instance", "users-db"),
	)

	// Simulate database query
	time.Sleep(time.Duration(20+rand.Intn(30)) * time.Millisecond)

	// Simulate memory allocation to generate heap profile data
	simulateMemoryAllocation()

	dbSpan.Finish()

	// Create a child span for cache operation
	cacheSpan, ctx := tracer.StartSpanFromContext(ctx, "cache.get",
		tracer.ResourceName("user:123"),
		tracer.Tag("cache.type", "redis"),
	)

	// Simulate cache lookup
	time.Sleep(time.Duration(5+rand.Intn(10)) * time.Millisecond)

	cacheSpan.Finish()

	// Create a child span for business logic
	logicSpan, _ := tracer.StartSpanFromContext(ctx, "business.process",
		tracer.ResourceName("process_user_data"),
	)

	// Simulate CPU-intensive work
	simulateCPUWork()

	logicSpan.Finish()

	// Send DogStatsD metrics
	sendDogStatsDMetrics()
}

func simulateMemoryAllocation() {
	// Allocate varying amounts of memory to generate heap profile data
	allocSize := 1024*1024 + rand.Intn(5*1024*1024) // 1-6MB random allocation
	data := make([]byte, allocSize)

	// Fill with some data to ensure memory is actually used
	for i := range data {
		data[i] = byte(i % 256)
	}

	// Allocate additional memory structures to simulate realistic memory patterns
	userRecords := make([]map[string]interface{}, 100)
	for i := range userRecords {
		userRecords[i] = map[string]interface{}{
			"id":    i,
			"name":  fmt.Sprintf("user-%d", i),
			"email": fmt.Sprintf("user%d@example.com", i),
			"data":  make([]byte, 1024), // 1KB per record
		}
	}

	// Keep references to prevent immediate GC
	_ = data
	_ = userRecords
}

func simulateCPUWork() {
	// Do some CPU work to generate CPU profile data
	start := time.Now()
	sum := 0
	for time.Since(start) < 20*time.Millisecond {
		for i := 0; i < 10000; i++ {
			sum += i * i
		}
	}
	_ = sum
}

// sendDogStatsDMetrics sends a few DogStatsD metrics using the Datadog library
func sendDogStatsDMetrics() {
	if statsdClient == nil {
		return
	}

	tags := []string{fmt.Sprintf("service:%s", *serviceName), fmt.Sprintf("env:%s", *env)}

	// Send a counter metric
	statsdClient.Count("request.count", 1, tags, 1)

	// Send a gauge metric with random value
	gaugeValue := float64(50 + rand.Intn(50))
	statsdClient.Gauge("request.duration", gaugeValue, tags, 1)

	// Send a histogram metric
	histogramValue := float64(10 + rand.Intn(100))
	statsdClient.Histogram("request.latency", histogramValue, tags, 1)

	// Send a set metric
	setValue := fmt.Sprintf("user-%d", rand.Intn(1000))
	statsdClient.Set("unique_users", setValue, tags, 1)
}

func init() {
	rand.Seed(time.Now().UnixNano())
	fmt.Println("Go Traced Application")
	fmt.Println("=====================")
}
