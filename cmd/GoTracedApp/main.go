package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"time"

	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
	"gopkg.in/DataDog/dd-trace-go.v1/profiler"
)

var (
	serviceName = flag.String("service", "go-traced-app", "Service name for traces and profiling")
	agentAddr   = flag.String("agent", "localhost:8126", "Datadog agent address")
	env         = flag.String("env", "dev", "Environment name")
)

func main() {
	flag.Parse()

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
	err := profiler.Start(
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

	log.Printf("Go Traced App started (service: %s)", *serviceName)
	log.Printf("Sending traces to %s", *agentAddr)
	log.Printf("Environment: %s", *env)
	log.Println("Press Ctrl+C to stop")

	// Run the demo workload
	runDemo()
}

func runDemo() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
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

	// Allocate some memory to generate heap profile data
	_ = make([]byte, 1024*1024) // 1MB allocation

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

func init() {
	rand.Seed(time.Now().UnixNano())
	fmt.Println("Go Traced Application")
	fmt.Println("=====================")
}
