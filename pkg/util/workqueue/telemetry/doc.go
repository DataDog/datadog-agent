// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

/*
Package telemetry is a utility package that provides helper methods for creating and registering metrics for kubernetes workqueue.

It can be used to create a new [MetricsProvider] object which can be used as a provider for a [workqueue].

You should not create multiple metrics providers and register them for the same workqueue.
It is recommended to create a global provider using the NewQueueMetricsProvider function and use it when creating the workqueue.

Example:

	// global variable
	queueMetricsProvider = workqueuetelemetry.NewQueueMetricsProvider()


	// initialise workqueue with the metrics provider
	wq := workqueue.NewRateLimitingQueueWithConfig(
			workqueue.NewItemExponentialFailureRateLimiter(
				time.Duration(2*time.Second),
				time.Duration(2*time.Minute),
			),
			workqueue.RateLimitingQueueConfig{
				Name:            "subsystem",
				MetricsProvider: queueMetricsProvider,
			},
		)

[MetricsProvdier] https://pkg.go.dev/k8s.io/client-go/util/workqueue#MetricsProvider
[workqueue] https://pkg.go.dev/k8s.io/client-go/util/workqueue
*/
package telemetry
