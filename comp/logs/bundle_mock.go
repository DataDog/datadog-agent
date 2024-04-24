// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package logs //nolint:revive // TODO(AML) Fix revive linter

import (
	"github.com/DataDog/datadog-agent/comp/logs/agent"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// MockBundle defines the mock fx options for this bundle.
func MockBundle() fxutil.BundleOptions {
	return fxutil.Bundle(
		agent.MockModule())
}
