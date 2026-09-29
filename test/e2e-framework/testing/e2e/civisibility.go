// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package e2e

import (
	"testing"
	_ "unsafe"

	_ "github.com/DataDog/dd-trace-go/v2/civisibility"
)

//go:linkname instrumentTestifySuiteRun github.com/DataDog/dd-trace-go/v2/internal/civisibility/integrations/gotesting.instrumentTestifySuiteRun
func instrumentTestifySuiteRun(*testing.T, any)
