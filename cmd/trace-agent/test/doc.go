// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package test provides utilities for running integration tests on the trace agent.
// You may use the runner to start a fake backend, a trace-agent instance with a custom
// configuration, post payloads to the agent and assert the results.
//
// To use this package, start by instantiating a runner. It needs not be initialized and can
// be used as is, for example:
//
//	// this runner is ready to use:
//	var runner test.Runner
//
// Next, start the fake backend before running any tests:
//
//	if err := runner.Start(); err != nil {
//		log.Fatal(err)
//	}
//
// Then, use `runner.RunAgent`, `runner.Post`, `runner.Out` and `runner.KillAgent` to run tests.
// For a full demonstration, see the package example.
package test
