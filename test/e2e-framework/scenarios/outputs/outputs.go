// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// package outputs contains the output types for the scenarios.
// This packageshould be very similar to the environments defined in the testing/environments package.
// The main difference is that these structs should not contain any dependency to clients that are actually used inside the tests.
// The goal is to allow the use of scenarios without pulling all the test client dependencies, scenarios for local and testing were first merged without that abstraction.
// But it lead to unnecesarily increasing the build time of the Pulumi binaries when running them locally, because pulling client used in test pull a lot of dependencies, especially zstd due to the usage of
// agent payload in the fakeintake client.
package outputs
