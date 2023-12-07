// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Module fakeintake provides a dummy [Server] implementation of Datadog intake, meant to be used with integration and e2e tests.
// Package [Client] implements an API to interact with a fakeintake server from go tests
// fakeintake server is available as a [docker container]
//
// [Server]: https://pkg.go.dev/github.com/DataDog/datadog-agent@main/test/fakeintake/server
// [Client]: https://pkg.go.dev/github.com/DataDog/datadog-agent@main/test/fakeintake/client
// [docker container]: https://hub.docker.com/r/datadog/fakeintake
//
//nolint:revive // TODO(APL) Fix revive linter
package fakeintake
