// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !(linux || windows)

// Package processeventconsumer provides the interface for a process event consumer
package processeventconsumer

// ProcessEventConsumer is a consumer of process events
type ProcessEventConsumer interface{}
