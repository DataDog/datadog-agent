// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ipc implement a basic Agent DNS to resolve Agent IPC addresses
// It would provide Client and Server building blocks to convert "http://core-cmd/agent/status" into "http://localhost:5001/agent/status" based on the configuration
package ipc
