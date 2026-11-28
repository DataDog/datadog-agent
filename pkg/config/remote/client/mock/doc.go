// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package mock provides a fake Remote Config client for tests.
// The fake records subscriptions and delivers updates on demand.
// It does not simulate the real client: there is no polling, TUF verification or signature expiry.
// The fake is only built with the `test` build tag.
package mock
