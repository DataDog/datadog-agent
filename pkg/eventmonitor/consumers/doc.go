// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package consumers contains consumers that can be readily used by other packages without
// having to implement the EventConsumerHandler interface manually:
// - ProcessConsumer (process.go): a consumer of process exec/exit events that can be subscribed to via callbacks
package consumers
