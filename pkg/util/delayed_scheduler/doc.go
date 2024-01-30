// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package delayedscheduler

/*
This package implements a scheduler that can be used to schedule the execution of a function at a given time.
It is similar to time.AfterFunc but it doesn't use more than one timer at a time.
Usage:

// Create a scheduler
scheduler := NewScheduler()

// Schedule some tasks
scheduler.Schedule(func() {
	fmt.Println("World")
}, time.Now().Add(2*time.Second))

scheduler.Schedule(func() {
	fmt.Println("Hello")
}, time.Now().Add(1*time.Second))

// Stop the scheduler
scheduler.Stop()
*/
