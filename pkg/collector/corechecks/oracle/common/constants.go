// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

// Package common contains common constant definitions.
package common

// IntegrationName is the name of the integration.
const IntegrationName = "oracle"

// IntegrationNameScheduler is the name of the integration for the scheduler.
const IntegrationNameScheduler = "oracle"

// Godror is the name of the godror driver which relies on an external Oracle client.
const Godror = "godror"

// GoOra is the name of the go-ora driver which is a pure Go implementation.
const GoOra = "oracle"
