// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package submitter implements a component to submit collected data in the Process Agent to
// supported Datadog intakes.
package submitter

// Mock implements mock-specific methods.
type Mock interface {
	Component
}
