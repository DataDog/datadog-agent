// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package health implements the internal healthcheck
package health

// Option represents the application of an option to a component's health check
type Option func(*component)

// Once has the effect of not checking the health of a component once it has been marked healthy once
func Once(c *component) {
	c.once = true
}
