// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package logslibrary provides the logs library component bundle with mock implementations
package logslibrary

import (
	batchsenderfx "github.com/DataDog/datadog-agent/comp/logs-library/api/batchsender/fx"
	kubehealthfx "github.com/DataDog/datadog-agent/comp/logs-library/kubehealth/fx"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func MockBundle() fxutil.BundleOptions {
	return fxutil.Bundle(
		batchsenderfx.MockModule(),
		kubehealthfx.MockModule(),
	)
}
