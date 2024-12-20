// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package common

import "testing"

// Context defines an interface that allows to get information about current test context
type Context interface {
	T() *testing.T
}

// Initializable defines the interface for an object that needs to be initialized
type Initializable interface {
	Init(Context) error
}
